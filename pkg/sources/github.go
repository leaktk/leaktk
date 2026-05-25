package sources

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/leaktk/leaktk/pkg/config"
	"github.com/leaktk/leaktk/pkg/logger"
	"github.com/leaktk/leaktk/pkg/proto"
	"github.com/leaktk/leaktk/pkg/sources/auths"
)

type gitHubEvent struct {
	Type string `json:"type"`
	Repo struct {
		URL string `json:"url"`
	} `json:"repo"`
	Payload struct {
		Ref     string `json:"ref"`
		RefType string `json:"ref_type"`
	} `json:"payload"`
}

type GitHub struct {
	id     string
	client *http.Client
	token  string
	etag   string
}

func NewGitHub(srcCfg config.Source) (Source, error) {
	auth, err := auths.NewAuth(srcCfg.Auth)
	if err != nil {
		return nil, err
	}

	github := GitHub{id: srcCfg.ID, client: &http.Client{}, token: auth.Token}

	return &github, nil
}

func (g *GitHub) ScanRequests(yield func(*proto.Request)) {
	poll := 60 * time.Second
	for {
		time.Sleep(poll)

		req, err := http.NewRequest("GET", "https://api.github.com/events?per_page=100", nil)
		if err != nil {
			logger.Error("could not create request: %v", err)
			continue
		}

		req.Header.Add("Authorization", "Bearer "+g.token)
		req.Header.Add("Accept", "application/vnd.github+json")
		if g.etag != "" {
			req.Header.Add("If-None-Match", g.etag)
		}

		resp, err := g.client.Do(req)
		if err != nil {
			logger.Error("could not fetch github events: %v", err)
			continue
		}

		if resp.StatusCode == 304 {
			resp.Body.Close()
			continue
		}

		g.etag = resp.Header.Get("ETag")

		newPoll := resp.Header.Get("X-Poll-Interval")
		if newPoll != "" {
			if seconds, err := strconv.Atoi(newPoll); err == nil {
				poll = time.Duration(seconds) * time.Second
			}
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			logger.Error("could not read response body: %v", err)
			continue
		}
		resp.Body.Close()

		var events []gitHubEvent
		if json.Unmarshal(body, &events) != nil {
			logger.Error("could not parse github events: %v", err)
			continue
		}

		for _, event := range events {
			cloneURL := strings.Replace(event.Repo.URL, "https://api.github.com/repos/", "https://github.com/", 1)
			if event.Type == "PushEvent" || (event.Type == "CreateEvent" && (event.Payload.RefType == "branch" || event.Payload.RefType == "tag")) {
				branch := strings.TrimPrefix(event.Payload.Ref, "refs/heads/")
				yield(&proto.Request{ID: g.id, Kind: proto.GitRepoRequestKind, Resource: cloneURL, Opts: proto.Opts{Branch: branch}})
			}
		}
	}
}
