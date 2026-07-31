package collectors

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/leaktk/leaktk/pkg/config"
)

func TestCompileExtractions(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		compiled, err := compileExtractions([]config.Extraction{
			{Attribute: "info", Pattern: regexp.MustCompile(`github\.com/(?P<Username>\w+)`), Kind: "GitHubAccount"},
		})
		require.NoError(t, err)
		require.Len(t, compiled, 1)
		assert.Equal(t, "info", compiled[0].attribute)
		assert.Equal(t, "GitHubAccount", compiled[0].kind)
		require.Len(t, compiled[0].captures, 1)
		assert.Equal(t, UsernameFactKind, compiled[0].captures[0].factKind)
	})

	t.Run("multiple capture groups sorted", func(t *testing.T) {
		compiled, err := compileExtractions([]config.Extraction{
			{Attribute: "info", Pattern: regexp.MustCompile(`(?P<Username>\w+)@(?P<Name>\w+)`), Kind: "Account"},
		})
		require.NoError(t, err)
		require.Len(t, compiled[0].captures, 2)
		// Name(5) < Username(9)
		assert.Equal(t, NameFactKind, compiled[0].captures[0].factKind)
		assert.Equal(t, UsernameFactKind, compiled[0].captures[1].factKind)
	})

	t.Run("unknown capture group name", func(t *testing.T) {
		_, err := compileExtractions([]config.Extraction{
			{Attribute: "info", Pattern: regexp.MustCompile(`(?P<Bogus>\w+)`), Kind: "Account"},
		})
		assert.ErrorContains(t, err, "unknown fact kind in extraction capture group")
	})
}

func TestExtract(t *testing.T) {
	t.Run("single match", func(t *testing.T) {
		compiled, err := compileExtractions([]config.Extraction{
			{Attribute: "info", Pattern: regexp.MustCompile(`github\.com/(?P<Username>\w+)`), Kind: "GitHubAccount"},
		})
		require.NoError(t, err)

		var facts []Fact
		eidOffset, err := extract(t.Context(), 1, 2, compiled, "https://github.com/jdoe", "test-src", func(f Fact) error {
			facts = append(facts, f)
			return nil
		})

		require.NoError(t, err)
		assert.Equal(t, uint32(3), eidOffset)

		actual := make(map[uint32]map[string]string)
		for _, f := range facts {
			m, ok := actual[f.EntityID]
			if !ok {
				m = make(map[string]string)
				actual[f.EntityID] = m
			}
			m[f.Kind.String()] = f.Value
		}

		assert.Equal(t, "GitHubAccount", actual[2]["Kind"])
		assert.Equal(t, "jdoe", actual[2]["Username"])
		assert.Equal(t, "test-src", actual[2]["SourceID"])
		assert.Equal(t, "2", actual[1]["RelatedEntityID"])
	})

	t.Run("no match", func(t *testing.T) {
		compiled, err := compileExtractions([]config.Extraction{
			{Attribute: "info", Pattern: regexp.MustCompile(`github\.com/(?P<Username>\w+)`), Kind: "GitHubAccount"},
		})
		require.NoError(t, err)

		var facts []Fact
		eidOffset, err := extract(t.Context(), 1, 2, compiled, "no urls here", "test-src", func(f Fact) error {
			facts = append(facts, f)
			return nil
		})

		require.NoError(t, err)
		assert.Equal(t, uint32(2), eidOffset)
		assert.Empty(t, facts)
	})

	t.Run("multiple extractions", func(t *testing.T) {
		compiled, err := compileExtractions([]config.Extraction{
			{Attribute: "info", Pattern: regexp.MustCompile(`github\.com/(?P<Username>\w+)`), Kind: "GitHubAccount"},
			{Attribute: "info", Pattern: regexp.MustCompile(`(?P<Username>\w+)\.github\.io`), Kind: "GitHubPagesAccount"},
		})
		require.NoError(t, err)

		var facts []Fact
		eidOffset, err := extract(t.Context(), 1, 2, compiled, "https://github.com/jdoe https://jdoe.github.io", "test-src", func(f Fact) error {
			facts = append(facts, f)
			return nil
		})

		require.NoError(t, err)
		assert.Equal(t, uint32(4), eidOffset)

		actual := make(map[uint32]map[string][]string)
		for _, f := range facts {
			m, ok := actual[f.EntityID]
			if !ok {
				m = make(map[string][]string)
				actual[f.EntityID] = m
			}
			m[f.Kind.String()] = append(m[f.Kind.String()], f.Value)
		}

		assert.Equal(t, []string{"GitHubAccount"}, actual[2]["Kind"])
		assert.Equal(t, []string{"jdoe"}, actual[2]["Username"])
		assert.Equal(t, []string{"GitHubPagesAccount"}, actual[3]["Kind"])
		assert.Equal(t, []string{"jdoe"}, actual[3]["Username"])
		assert.Equal(t, []string{"2", "3"}, actual[1]["RelatedEntityID"])
	})

	t.Run("multiple matches same pattern", func(t *testing.T) {
		compiled, err := compileExtractions([]config.Extraction{
			{Attribute: "info", Pattern: regexp.MustCompile(`github\.com/(?P<Username>\w+)`), Kind: "GitHubAccount"},
		})
		require.NoError(t, err)

		var facts []Fact
		eidOffset, err := extract(t.Context(), 1, 2, compiled, "github.com/alice github.com/bob", "test-src", func(f Fact) error {
			facts = append(facts, f)
			return nil
		})

		require.NoError(t, err)
		assert.Equal(t, uint32(4), eidOffset)

		actual := make(map[uint32]map[string]string)
		for _, f := range facts {
			m, ok := actual[f.EntityID]
			if !ok {
				m = make(map[string]string)
				actual[f.EntityID] = m
			}
			m[f.Kind.String()] = f.Value
		}

		assert.Equal(t, "alice", actual[2]["Username"])
		assert.Equal(t, "bob", actual[3]["Username"])
	})
}
