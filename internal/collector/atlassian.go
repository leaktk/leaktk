package collector

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	nethttp "net/http"

	"github.com/leaktk/leaktk/pkg/config"
	"github.com/leaktk/leaktk/pkg/http"
	"github.com/leaktk/leaktk/pkg/logger"
)

const atlassianAdminURL = "https://api.atlassian.com/admin"

func atlassianCloudAdminDirIDs(ctx context.Context, baseURL string, src *config.AtlassianCloudAdminSource) (ids []string, err error) {
	var req *nethttp.Request
	var resp *nethttp.Response
	var nextCur string
	var url string

	client := http.NewClient()

	for {
		url = fmt.Sprintf("%s/v2/orgs/%s/directories?limit=100", baseURL, src.OrgID)
		if len(nextCur) != 0 {
			url += "&cursor=" + nextCur
		}

		req, err = nethttp.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			err = fmt.Errorf("failed to fetch directories: %w", err)
			goto done
		}
		req.Header.Set("Accept", "application/json")
		if err = src.SetHeader(req.Header); err != nil {
			err = fmt.Errorf("failed to fetch directories: %w", err)
			goto done
		}

		resp, err = client.Do(req)
		if err != nil {
			err = fmt.Errorf("failed to fetch directories: %w", err)
			goto done
		}

		if resp.StatusCode != 200 {
			body, _ := io.ReadAll(resp.Body)
			logger.Debug("atlassian directories fetch response body: %s", string(body))
			err = fmt.Errorf("failed to fetch directories: unexpected status status_code=%d", resp.StatusCode)
			goto done
		}

		var respData struct {
			Data []struct {
				ID string `json:"directoryId"`
			} `json:"data"`
			Links struct {
				Next string `json:"next"`
			} `json:"links"`
		}

		decoder := json.NewDecoder(bufio.NewReader(resp.Body))
		if err = decoder.Decode(&respData); err != nil {
			err = fmt.Errorf("could not decode directories: %w", err)
			goto done
		}

		for _, item := range respData.Data {
			if len(item.ID) > 0 {
				ids = append(ids, item.ID)
			}
		}

		nextCur = respData.Links.Next
		if len(nextCur) == 0 {
			goto done
		}
	}

done:
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	return ids, err
}

func atlassianCloudAdminFacts(ctx context.Context, src *config.AtlassianCloudAdminSource, _ func(fact Fact, err error) error) error {
	_, err := atlassianCloudAdminDirIDs(ctx, atlassianAdminURL, src)
	if err != nil {
		return fmt.Errorf("could not lookup atlassian cloud admin facts: %w", err)
	}
	return errors.New("not implemented") // TODO
}
