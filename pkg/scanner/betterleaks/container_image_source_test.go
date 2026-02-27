package betterleaks

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/betterleaks/betterleaks/sources"
	"github.com/fatih/semgroup"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestContainerImage(t *testing.T) {
	theFuture := time.Now().Add(time.Hour)
	containerImage := &ContainerImage{
		RawImageRef: "docker://quay.io/leaktk/fake-leaks:v2",
		Arch:        "amd64",
		Depth:       1,
		Sema:        semgroup.NewGroup(context.Background(), 1),
		Since:       &theFuture,
	}

	t.Run("FragmentsNormal", func(t *testing.T) {
		var (
			mu        sync.Mutex
			fragments []sources.Fragment
		)
		err := containerImage.Fragments(context.Background(), func(fragment sources.Fragment, err error) error {
			mu.Lock()
			defer mu.Unlock()
			fragments = append(fragments, fragment)
			return errors.New("only need one fragment to test")
		})

		require.Error(t, err)
		require.Len(t, fragments, 1)
		assert.Equal(t, "Fake Leaks", fragments[0].CommitInfo.AuthorName)
		assert.Equal(t, "fake-leaks@leaktk.org", fragments[0].CommitInfo.AuthorEmail)
	})

	t.Run("CallbackError", func(t *testing.T) {
		var called bool
		err := containerImage.Fragments(context.Background(), func(fragment sources.Fragment, err error) error {
			called = true
			return errors.New("test error")
		})

		require.Error(t, err)
		assert.True(t, called, "callback should be called at least once")
	})

	t.Run("PartialFragments", func(t *testing.T) {
		var (
			mu        sync.Mutex
			fragments []sources.Fragment
			calls     int
		)
		_ = containerImage.Fragments(context.Background(), func(fragment sources.Fragment, err error) error {
			mu.Lock()
			defer mu.Unlock()

			fragments = append(fragments, fragment)
			calls++
			if calls < 2 {
				return nil // allow more than one fragment
			}
			return errors.New("stop after two fragments")
		})

		assert.GreaterOrEqual(t, len(fragments), 2, "should collect at least two fragments if available")
	})
}
