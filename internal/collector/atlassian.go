package collector

import (
	"context"
	"errors"

	"github.com/leaktk/leaktk/pkg/config"
)

func atlassianCloudAdminFacts(_ context.Context, src *config.AtlassianCloudAdminSource, yield func(fact Fact, err error) error) error {
	return errors.New("not implemented") // TODO
}
