package gitleaks

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/fatih/semgroup"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zricethezav/gitleaks/v8/sources"
)

func TestContainerImage(t *testing.T) {
	theFuture := time.Now().Add(time.Hour)
	containerImage := &ContainerImage{
		RawImageRef: "docker://quay.io/leaktk/fake-leaks:v1.0.1",
		Arch:        "amd64",
		Depth:       1,
		Sema:        semgroup.NewGroup(context.Background(), 1),
		Since:       &theFuture,
	}

	fragments := []sources.Fragment{}
	err := containerImage.Fragments(context.Background(), func(fragment sources.Fragment, err error) error {
		fragments = append(fragments, fragment)

		return errors.New("only need one fragment to test")
	})

	require.NoError(t, err)
	assert.Len(t, fragments, 1)
	assert.Equal(t, "Fake Leaks", fragments[0].CommitInfo.AuthorName)
	assert.Equal(t, "fake-leaks@leaktk.org", fragments[0].CommitInfo.AuthorEmail)
}
