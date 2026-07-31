package collector

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	nethttp "net/http"
	"time"

	"github.com/leaktk/leaktk/pkg/config"
	"github.com/leaktk/leaktk/pkg/http"
	"github.com/leaktk/leaktk/pkg/logger"
)

func atlassianReq(ctx context.Context, src *config.AtlassianCloudAdminSource, client *nethttp.Client, method, url string, body io.Reader, respData any) error {
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
		return fmt.Errorf("unexpected status status_code=%d", resp.StatusCode)
	}

	decoder := json.NewDecoder(bufio.NewReader(resp.Body))
	if err = decoder.Decode(respData); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	return nil
}

func atlassianCloudAdminDirIDs(ctx context.Context, src *config.AtlassianCloudAdminSource) (dirIDs []string, err error) {
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

func atlassianCloudAdminYieldUserFacts(ctx context.Context, src *config.AtlassianCloudAdminSource, eidOffset uint32, dirID string, yield FactYieldFunc) (uint32, error) {
	var err error
	var jsonText []byte
	var payload struct {
		Limit  int    `json:"limit"`
		Cursor string `json:"cursor"`
	}

	fact := Fact{Timestamp: time.Now().Unix()}
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
			if item.Status == "active" {
				err = yieldKV(fact, ActiveFactKind, FactTrueValue, yield, err)
			} else {
				err = yieldKV(fact, ActiveFactKind, FactFalseValue, yield, err)
			}
			if item.EmailVerified {
				err = yieldKV(fact, EmailAddressVerifiedFactKind, FactTrueValue, yield, err)
			} else {
				err = yieldKV(fact, EmailAddressVerifiedFactKind, FactFalseValue, yield, err)
			}
			err = yieldKV(fact, SourceIDFactKind, src.ID(), yield, err)
			err = yieldKV(fact, IDFactKind, item.ID, yield, err)
			err = yieldKV(fact, EmailAddressFactKind, item.Email, yield, err)
			err = yieldKV(fact, NameFactKind, item.Name, yield, err)
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

func atlassianCloudAdminFacts(ctx context.Context, src *config.AtlassianCloudAdminSource, eidOffset uint32, yield FactYieldFunc) (uint32, error) {
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
