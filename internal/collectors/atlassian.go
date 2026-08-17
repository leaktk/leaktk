package collectors

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	nethttp "net/http"

	"github.com/leaktk/leaktk/internal/facts"
	"github.com/leaktk/leaktk/internal/sources"
	"github.com/leaktk/leaktk/pkg/http"
	"github.com/leaktk/leaktk/pkg/logger"
)

func atlassianReq(ctx context.Context, src *sources.AtlassianCloudAdmin, client *nethttp.Client, method, url string, body io.Reader, respData any) error {
	req, err := nethttp.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return fmt.Errorf("failed create request: %w", err)
	}

	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if err = src.SetHeader(req.Header); err != nil {
		return err
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		logger.Debug("atlassian admin API response body: %s", string(body))
		return fmt.Errorf("unexpected status status_code=%d url=%s", resp.StatusCode, req.URL)
	}

	decoder := json.NewDecoder(bufio.NewReader(resp.Body))
	if err = decoder.Decode(respData); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	logger.Trace("atlassian admin API response data: %+v", respData)

	return nil
}

func atlassianCloudAdminDirIDs(ctx context.Context, src *sources.AtlassianCloudAdmin) (dirIDs []string, err error) {
	nextCur := ""
	client := http.NewClient()

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
	client := http.NewClient()
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
			fact.EntityID = eidOffset
			eidOffset++

			if len(item.ID) == 0 {
				continue
			}
			err = facts.YieldWithKV(fact, facts.IDKind, item.ID, err, yield)
			if item.Status == "active" {
				err = facts.YieldWithKV(fact, facts.ActiveKind, facts.FactValueTrue, err, yield)
			} else {
				err = facts.YieldWithKV(fact, facts.ActiveKind, facts.FactValueFalse, err, yield)
			}
			err = facts.YieldWithKV(fact, facts.EmailAddressKind, item.Email, err, yield)
			if item.EmailVerified {
				err = facts.YieldWithKV(fact, facts.EmailAddressVerifiedKind, facts.FactValueTrue, err, yield)
			} else {
				err = facts.YieldWithKV(fact, facts.EmailAddressVerifiedKind, facts.FactValueFalse, err, yield)
			}
			err = facts.YieldWithKV(fact, facts.EntityKindKind, AtlassianCloudUserKind.String(), err, yield)
			err = facts.YieldWithKV(fact, facts.NameKind, item.Name, err, yield)
			err = facts.YieldWithKV(fact, facts.SourceIDKind, src.ID(), err, yield)
			err = facts.YieldWithKV(fact, facts.URLKind, fmt.Sprintf("https://home.atlassian.com/o/%s/people/%s", src.OrgID, item.ID), err, yield)
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
