package collector

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/leaktk/leaktk/pkg/config"
	"github.com/leaktk/leaktk/pkg/http"
)

const atlassianAdminURL = "https://api.atlassian.com/admin"

func atlassianCloudAdminDirIDs(ctx context.Context, src *config.AtlassianCloudAdminSource) ([]string, error) {
	client := http.NewClient()
	ids := make([]string, 0, 1)
	nextCur := ""

	for {
		url := fmt.Sprintf("%s/v2/orgs/%s/directories?limit=100", atlassianAdminURL, src.OrgID)
		if len(nextCur) != 0 {
			url += "&cursor=" + nextCur
		}

		req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch directories: %w", err)
		}
		req.Header.Set("Accept", "application/json")
		if err := src.SetHeader(req.Header); err != nil {
			return nil, fmt.Errorf("failed to fetch directories: %w", err)
		}

		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch directories: %w", err)
		}

		if resp.StatusCode != 200 {
			_ = resp.Body.Close()
			return nil, fmt.Errorf("failed to fetch directories: unexpected status status_code=%d", resp.StatusCode)
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
		if err := decoder.Decode(&respData); err != nil {
			_ = resp.Body.Close()
			return nil, fmt.Errorf("could not decode directories: %w", err)
		}

		for _, item := range respData.Data {
			if len(item.ID) > 0 {
				ids = append(ids, item.ID)
			}
		}

		nextCur = respData.Links.Next
		if len(nextCur) == 0 {
			_ = resp.Body.Close()
			return ids, nil
		}
	}
}

func atlassianCloudAdminFacts(ctx context.Context, src *config.AtlassianCloudAdminSource, _ func(fact Fact, err error) error) error {
	_, err := atlassianCloudAdminDirIDs(ctx, src)
	if err != nil {
		return fmt.Errorf("could not lookup atlassian cloud admin facts: %w", err)
	}
	return errors.New("not implemented") // TODO
}
