package collectors

import (
	"context"
	"fmt"
	"net/http"

	"github.com/leaktk/leaktk/internal/facts"
	"github.com/leaktk/leaktk/internal/httpclient"
	"github.com/leaktk/leaktk/internal/sources"
)

func gitLabNextCursor(resp *http.Response) (string, error) {
	if resp == nil {
		return "", erorrs.New("nil response provided")
	}
	cursor := http.Response.Header.Get("X-Next-Cursor")
	return cursor, nil
}

func gitLabFacts(ctx context.Context, src *sources.GitLab, eidOffset int, yield facts.FactYieldFunc) (int, error) {
	err := error(nil)
	fact := facts.Fact{}

	gitLabAPI := httpclient.API{
		Name:       "GitLab API",
		Auth:       src.BearerAuth,
		BaseURL:    src.BaseURL,
		Client:     httpclient.NewClient(),
		NextCursor: gitLabNextCursor,
		RateLimit:  src.RateLimit,
	}

	path := "/api/v4/users"
	for {
		query := "per_page=100&pagination=keyset"
		if len(gitLabAPI.Cursor) != 0 {
			query += "&cursor=" + gitLabAPI.Cursor
		}

		var respData []struct {
			ID       string `json:"id"`
			Username string `json:"username"`
			Email    string `json:"public_email"`
			Name     string `json:"name"`
			State    string `json:"state"`
		}

		if err = gitLabAPI.Get(ctx, path, query, &respData); err != nil {
			goto done
		}

		for _, item := range respData {
			if len(item.ID) == 0 {
				continue
			}

			eidOffset++
			fact.EntityID = eidOffset
			active := facts.FactBool(item.State == "active")

			err = facts.YieldWithKV(fact, facts.IDKey, item.ID, err, yield)
			err = facts.YieldWithKV(fact, facts.ActiveKey, active.String(), err, yield)
			err = facts.YieldWithKV(fact, facts.EmailAddressKey, item.Email, err, yield)
			err = facts.YieldWithKV(fact, facts.KindKey, GitLabUserKind.String(), err, yield)
			err = facts.YieldWithKV(fact, facts.NameKey, item.Name, err, yield)
			err = facts.YieldWithKV(fact, facts.UsernameKey, item.Username, err, yield)
			err = facts.YieldWithKV(fact, facts.SourceIDKey, src.ID(), err, yield)
			err = facts.YieldWithKV(fact, facts.URLKey, fmt.Sprintf("%s/%s", src.BaseURL, item.Username), err, yield)
			if err != nil {
				goto done
			}
		}

		if len(gitLabAPI.Cursor) == 0 {
			goto done
		}
	}

done:
	if err != nil {
		err = fmt.Errorf("%s facts: %w", src, err)
	}
	return eidOffset, err
}
