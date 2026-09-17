package collectors

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/leaktk/leaktk/internal/facts"
	"github.com/leaktk/leaktk/internal/httpclient"
	"github.com/leaktk/leaktk/internal/sources"
	"github.com/leaktk/leaktk/pkg/logger"
)

func atlassianReq(ctx context.Context, src *sources.AtlassianCloudAdmin, client *http.Client, method, url string, reqBody io.Reader, respData any) error {
	var (
		err  error
		req  *http.Request
		resp *http.Response
	)

	for resp == nil || resp.StatusCode == http.StatusTooManyRequests {
		req, err = http.NewRequestWithContext(ctx, method, url, reqBody)
		if err != nil {
			return fmt.Errorf("failed create request: %w", err)
		}

		req.Header.Set("Accept", "application/json")
		if reqBody != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		if err = src.SetHeader(req.Header); err != nil {
			return err
		}

		// Wait if needed
		if err := src.RateLimit.Wait(ctx, req); err != nil {
			return fmt.Errorf("error rate limiting requests: %w url=%s", err, req.URL)
		}

		resp, err = client.Do(req)
		if err != nil {
			return fmt.Errorf("request failed: %w", err)
		}

		// Update the rate limit based on the server's response
		src.RateLimit.Update(resp)
	}

	respBody, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		return fmt.Errorf("could not read complete resp body: %w", err)
	}

	if resp.StatusCode == 200 {
		if err = json.Unmarshal(respBody, respData); err != nil {
			logger.Trace("atlassian admin API response body: %s", string(respBody))
			return fmt.Errorf("decode response: %w", err)
		}
		logger.Trace("atlassian admin API response data: %+v", respData)
		return nil
	}

	logger.Trace("atlassian admin API response body: %s", string(respBody))
	return fmt.Errorf("unexpected status status_code=%d url=%s", resp.StatusCode, req.URL)
}

func atlassianCloudAdminDirIDs(ctx context.Context, src *sources.AtlassianCloudAdmin) (dirIDs []string, err error) {
	nextCur := ""
	client := httpclient.NewClient()

	for {
		url := fmt.Sprintf("%s/v2/orgs/%s/directories?limit=100", src.BaseURL, src.OrgID)
		if len(nextCur) != 0 {
			url += "&cursor=" + nextCur
		}

		var respData struct {
			Data []struct {
				ID string `json:"directoryId"`
			} `json:"data"`
			Links struct {
				Next string `json:"next"`
			} `json:"links"`
		}

		if err = atlassianReq(ctx, src, client, "GET", url, nil, &respData); err != nil {
			goto done
		}

		for _, item := range respData.Data {
			if len(item.ID) > 0 {
				dirIDs = append(dirIDs, item.ID)
			}
		}

		nextCur = respData.Links.Next
		if len(nextCur) == 0 {
			goto done
		}
	}

done:
	if err != nil {
		err = fmt.Errorf("fetch directories: %w", err)
	}
	return dirIDs, err
}

func atlassianCloudAdminYieldUserFacts(ctx context.Context, src *sources.AtlassianCloudAdmin, eidOffset int, dirID string, yield facts.FactYieldFunc) (int, error) {
	var err error
	var jsonText []byte
	var payload struct {
		Limit  int    `json:"limit"`
		Cursor string `json:"cursor,omitempty"`
	}

	fact := facts.Fact{}
	client := httpclient.NewClient()
	payload.Limit = 100

	for {
		url := fmt.Sprintf("%s/v2/orgs/%s/directories/%s/users/search", src.BaseURL, src.OrgID, dirID)
		if jsonText, err = json.Marshal(&payload); err != nil {
			goto done
		}

		var respData struct {
			Data []struct {
				ID            string `json:"accountId"`
				Status        string `json:"status"`
				Name          string `json:"name"`
				Email         string `json:"email"`
				EmailVerified bool   `json:"emailVerified"`
			} `json:"data"`
			Links struct {
				Next string `json:"next"`
			} `json:"links"`
		}
		if err = atlassianReq(ctx, src, client, "POST", url, bytes.NewBuffer(jsonText), &respData); err != nil {
			goto done
		}

		for _, item := range respData.Data {
			if len(item.ID) == 0 {
				continue
			}

			eidOffset++
			fact.EntityID = eidOffset
			active := facts.FactBool(item.Status == "active")
			emailVerified := facts.FactBool(item.EmailVerified)

			err = facts.YieldWithKV(fact, facts.IDKey, item.ID, err, yield)
			err = facts.YieldWithKV(fact, facts.ActiveKey, active.String(), err, yield)
			err = facts.YieldWithKV(fact, facts.EmailAddressKey, item.Email, err, yield)
			err = facts.YieldWithKV(fact, facts.EmailAddressVerifiedKey, emailVerified.String(), err, yield)
			err = facts.YieldWithKV(fact, facts.KindKey, AtlassianCloudUserKind.String(), err, yield)
			err = facts.YieldWithKV(fact, facts.NameKey, item.Name, err, yield)
			err = facts.YieldWithKV(fact, facts.SourceIDKey, src.ID(), err, yield)
			err = facts.YieldWithKV(fact, facts.URLKey, fmt.Sprintf("https://home.atlassian.com/o/%s/people/%s", src.OrgID, item.ID), err, yield)

			if err != nil {
				goto done
			}
		}

		payload.Cursor = respData.Links.Next
		if len(payload.Cursor) == 0 {
			goto done
		}
	}

done:
	if err != nil {
		err = fmt.Errorf("search users: %w", err)
	}
	return eidOffset, err
}

func atlassianCloudAdminFacts(ctx context.Context, src *sources.AtlassianCloudAdmin, eidOffset int, yield facts.FactYieldFunc) (int, error) {
	dirIDs, err := atlassianCloudAdminDirIDs(ctx, src)
	if err != nil {
		return eidOffset, fmt.Errorf("atlassian cloud admin facts: %w", err)
	}
	for _, dirID := range dirIDs {
		if eidOffset, err = atlassianCloudAdminYieldUserFacts(ctx, src, eidOffset, dirID, yield); err != nil {
			return eidOffset, err
		}
	}
	return eidOffset, nil
}
