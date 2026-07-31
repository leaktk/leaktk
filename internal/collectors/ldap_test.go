package collectors

// import (
// 	"regexp"
// 	"testing"
//
// 	"github.com/go-ldap/ldap/v3"
// 	"github.com/stretchr/testify/assert"
// 	"github.com/stretchr/testify/require"
//
// 	"github.com/leaktk/leaktk/pkg/config"
// )
//
// func TestParseLDAPScope(t *testing.T) {
// 	tests := []struct {
// 		input    string
// 		expected int
// 	}{
// 		{"base", ldap.ScopeBaseObject},
// 		{"one", ldap.ScopeSingleLevel},
// 		{"sub", ldap.ScopeWholeSubtree},
// 	}
//
// 	for _, tt := range tests {
// 		t.Run(tt.input, func(t *testing.T) {
// 			scope, err := parseLDAPScope(tt.input)
// 			require.NoError(t, err)
// 			assert.Equal(t, tt.expected, scope)
// 		})
// 	}
//
// 	t.Run("invalid", func(t *testing.T) {
// 		_, err := parseLDAPScope("invalid")
// 		assert.ErrorContains(t, err, "unknown LDAP scope")
// 	})
// }
//
// func TestResolveAttrMappings(t *testing.T) {
// 	t.Run("valid", func(t *testing.T) {
// 		mappings, err := resolveAttrMappings(map[string]string{
// 			"cn":   "Name",
// 			"uuid": "ID",
// 			"uid":  "Username",
// 			"mail": "EmailAddress",
// 		})
// 		require.NoError(t, err)
// 		require.Len(t, mappings, 4)
//
// 		// Should be sorted by facts.Kind ascending: ID(0), EmailAddress(2), Name(5), Username(9)
// 		assert.Equal(t, facts.IDKind, mappings[0].factKind)
// 		assert.Equal(t, "uuid", mappings[0].ldapAttr)
// 		assert.Equal(t, facts.EmailAddressKind, mappings[1].factKind)
// 		assert.Equal(t, "mail", mappings[1].ldapAttr)
// 		assert.Equal(t, facts.NameKind, mappings[2].factKind)
// 		assert.Equal(t, "cn", mappings[2].ldapAttr)
// 		assert.Equal(t, facts.UsernameKind, mappings[3].factKind)
// 		assert.Equal(t, "uid", mappings[3].ldapAttr)
// 	})
//
// 	t.Run("unknown fact kind", func(t *testing.T) {
// 		_, err := resolveAttrMappings(map[string]string{
// 			"cn": "DoesNotExist",
// 		})
// 		assert.ErrorContains(t, err, "unknown fact kind in attributes")
// 	})
//
// 	t.Run("empty", func(t *testing.T) {
// 		mappings, err := resolveAttrMappings(map[string]string{})
// 		require.NoError(t, err)
// 		assert.Empty(t, mappings)
// 	})
// }
//
// func TestLDAPEntryURL(t *testing.T) {
// 	tests := []struct {
// 		name      string
// 		serverURL string
// 		dn        string
// 		expected  string
// 	}{
// 		{
// 			name:      "simple",
// 			serverURL: "ldaps://ldap.example.com:636",
// 			dn:        "uid=jdoe,ou=people,dc=example,dc=com",
// 			expected:  "ldaps://ldap.example.com:636/uid=jdoe%2Cou=people%2Cdc=example%2Cdc=com",
// 		},
// 		{
// 			name:      "special characters",
// 			serverURL: "ldap://ldap.example.com",
// 			dn:        "cn=John Doe+uid=jdoe,ou=people,dc=example,dc=com",
// 			expected:  "ldap://ldap.example.com/cn=John%20Doe+uid=jdoe%2Cou=people%2Cdc=example%2Cdc=com",
// 		},
// 	}
//
// 	for _, tt := range tests {
// 		t.Run(tt.name, func(t *testing.T) {
// 			assert.Equal(t, tt.expected, ldapEntryURL(tt.serverURL, tt.dn))
// 		})
// 	}
// }
//
// func TestYieldLDAPEntryFacts(t *testing.T) {
// 	mappings := []attrMapping{
// 		{ldapAttr: "uuid", factKind: facts.IDKind},
// 		{ldapAttr: "mail", factKind: facts.EmailAddressKind},
// 		{ldapAttr: "cn", factKind: facts.NameKind},
// 		{ldapAttr: "uid", factKind: facts.UsernameKind},
// 	}
//
// 	entries := []*ldap.Entry{
// 		ldap.NewEntry("uid=jdoe,ou=people,dc=example,dc=com", map[string][]string{
// 			"uuid": {"550e8400-e29b-41d4-a716-446655440000"},
// 			"uid":  {"jdoe"},
// 			"mail": {"jdoe@example.com"},
// 			"cn":   {"John Doe"},
// 		}),
// 		ldap.NewEntry("uid=asmith,ou=people,dc=example,dc=com", map[string][]string{
// 			"uuid": {"550e8400-e29b-41d4-a716-446655440001"},
// 			"uid":  {"asmith"},
// 			"cn":   {"Alice Smith"},
// 		}),
// 	}
//
// 	var facts []Fact
// 	eidOffset, err := yieldLDAPEntryFacts(t.Context(), entries, mappings, nil, "test-ldap", "ldaps://ldap.example.com:636", 1, func(fact Fact) error {
// 		facts = append(facts, fact)
// 		return nil
// 	})
//
// 	require.NoError(t, err)
// 	assert.Equal(t, uint32(3), eidOffset)
//
// 	actual := make(map[uint32]map[string]string)
// 	for _, fact := range facts {
// 		emap, ok := actual[fact.EntityID]
// 		if !ok {
// 			emap = make(map[string]string)
// 			actual[fact.EntityID] = emap
// 		}
// 		emap[fact.Kind.String()] = fact.Value
// 	}
//
// 	assert.Equal(t, map[uint32]map[string]string{
// 		1: {
// 			facts.IDKind.String():           "550e8400-e29b-41d4-a716-446655440000",
// 			facts.EmailAddressKind.String(): "jdoe@example.com",
// 			facts.NameKind.String():         "John Doe",
// 			facts.SourceIDKind.String():     "test-ldap",
// 			facts.URLKind.String():          "ldaps://ldap.example.com:636/uid=jdoe%2Cou=people%2Cdc=example%2Cdc=com",
// 			facts.UsernameKind.String():     "jdoe",
// 		},
// 		2: {
// 			facts.IDKind.String():       "550e8400-e29b-41d4-a716-446655440001",
// 			facts.NameKind.String():     "Alice Smith",
// 			facts.SourceIDKind.String(): "test-ldap",
// 			facts.URLKind.String():      "ldaps://ldap.example.com:636/uid=asmith%2Cou=people%2Cdc=example%2Cdc=com",
// 			facts.UsernameKind.String(): "asmith",
// 		},
// 	}, actual)
// }
//
// func TestYieldLDAPEntryFactsOrdering(t *testing.T) {
// 	mappings := []attrMapping{
// 		{ldapAttr: "uuid", factKind: facts.IDKind},
// 		{ldapAttr: "mail", factKind: facts.EmailAddressKind},
// 		{ldapAttr: "cn", factKind: facts.NameKind},
// 		{ldapAttr: "uid", factKind: facts.UsernameKind},
// 	}
//
// 	entries := []*ldap.Entry{
// 		ldap.NewEntry("uid=jdoe,ou=people,dc=example,dc=com", map[string][]string{
// 			"uuid": {"550e8400-e29b-41d4-a716-446655440000"},
// 			"uid":  {"jdoe"},
// 			"mail": {"jdoe@example.com"},
// 			"cn":   {"John Doe"},
// 		}),
// 	}
//
// 	var kinds []facts.Kind
// 	_, err := yieldLDAPEntryFacts(t.Context(), entries, mappings, nil, "test-ldap", "ldaps://ldap.example.com:636", 1, func(fact Fact) error {
// 		kinds = append(kinds, fact.Kind)
// 		return nil
// 	})
//
// 	require.NoError(t, err)
//
// 	// Mapped facts in ascending facts.Kind order, then implicit facts (SourceID, URL)
// 	assert.Equal(t, []facts.Kind{
// 		facts.IDKind,
// 		facts.EmailAddressKind,
// 		facts.NameKind,
// 		facts.UsernameKind,
// 		facts.SourceIDKind,
// 		facts.URLKind,
// 	}, kinds)
// }
//
// func TestYieldLDAPEntryFactsEmpty(t *testing.T) {
// 	mappings := []attrMapping{
// 		{ldapAttr: "uid", factKind: facts.IDKind},
// 	}
//
// 	eidOffset, err := yieldLDAPEntryFacts(t.Context(), nil, mappings, nil, "test-ldap", "ldaps://ldap.example.com:636", 5, func(fact Fact) error {
// 		t.Fatal("yield should not be called for empty entries")
// 		return nil
// 	})
//
// 	require.NoError(t, err)
// 	assert.Equal(t, uint32(5), eidOffset)
// }
//
// func TestYieldLDAPEntryFactsWithExtractions(t *testing.T) {
// 	mappings := []attrMapping{
// 		{ldapAttr: "uuid", factKind: facts.IDKind},
// 		{ldapAttr: "uid", factKind: facts.UsernameKind},
// 	}
//
// 	extractions, err := compileExtractions([]config.Extraction{
// 		{Attribute: "info", Pattern: regexp.MustCompile(`github\.com/(?P<Username>\w+)`), Kind: "GitHubAccount"},
// 		{Attribute: "info", Pattern: regexp.MustCompile(`(?P<Username>\w+)\.github\.io`), Kind: "GitHubPagesAccount"},
// 		{Attribute: "info", Pattern: regexp.MustCompile(`gitlab\.com/(?P<Username>[\w.-]+)`), Kind: "GitLabAccount"},
// 	})
// 	require.NoError(t, err)
//
// 	entries := []*ldap.Entry{
// 		ldap.NewEntry("uid=jdoe,ou=people,dc=example,dc=com", map[string][]string{
// 			"uuid": {"550e8400-e29b-41d4-a716-446655440000"},
// 			"uid":  {"jdoe"},
// 			"info": {"https://github.com/jdoe https://jdoe.github.io https://gitlab.com/jdoe"},
// 		}),
// 	}
//
// 	var facts []Fact
// 	eidOffset, err := yieldLDAPEntryFacts(t.Context(), entries, mappings, extractions, "test-ldap", "ldaps://ldap.example.com:636", 1, func(fact Fact) error {
// 		facts = append(facts, fact)
// 		return nil
// 	})
//
// 	require.NoError(t, err)
// 	// parent(1) + 3 children(2,3,4)
// 	assert.Equal(t, uint32(5), eidOffset)
//
// 	actual := make(map[uint32]map[string][]string)
// 	for _, fact := range facts {
// 		emap, ok := actual[fact.EntityID]
// 		if !ok {
// 			emap = make(map[string][]string)
// 			actual[fact.EntityID] = emap
// 		}
// 		emap[fact.Kind.String()] = append(emap[fact.Kind.String()], fact.Value)
// 	}
//
// 	// Parent entity
// 	parent := actual[1]
// 	assert.Equal(t, []string{"550e8400-e29b-41d4-a716-446655440000"}, parent["ID"])
// 	assert.Equal(t, []string{"jdoe"}, parent["Username"])
// 	assert.Equal(t, []string{"2", "3", "4"}, parent["RelatedEntityID"])
//
// 	// GitHub account child
// 	github := actual[2]
// 	assert.Equal(t, []string{"GitHubAccount"}, github["Kind"])
// 	assert.Equal(t, []string{"jdoe"}, github["Username"])
// 	assert.Equal(t, []string{"test-ldap"}, github["SourceID"])
//
// 	// GitHub Pages child
// 	ghPages := actual[3]
// 	assert.Equal(t, []string{"GitHubPagesAccount"}, ghPages["Kind"])
// 	assert.Equal(t, []string{"jdoe"}, ghPages["Username"])
//
// 	// GitLab child
// 	gitlab := actual[4]
// 	assert.Equal(t, []string{"GitLabAccount"}, gitlab["Kind"])
// 	assert.Equal(t, []string{"jdoe"}, gitlab["Username"])
// }
//
// func TestYieldLDAPEntryFactsNoExtractionMatch(t *testing.T) {
// 	extractions, err := compileExtractions([]config.Extraction{
// 		{Attribute: "info", Pattern: regexp.MustCompile(`github\.com/(?P<Username>\w+)`), Kind: "GitHubAccount"},
// 	})
// 	require.NoError(t, err)
//
// 	entries := []*ldap.Entry{
// 		ldap.NewEntry("uid=jdoe,ou=people,dc=example,dc=com", map[string][]string{
// 			"uid":  {"jdoe"},
// 			"info": {"no urls here"},
// 		}),
// 	}
//
// 	var facts []Fact
// 	_, err = yieldLDAPEntryFacts(t.Context(), entries, nil, extractions, "test-ldap", "ldaps://ldap.example.com:636", 1, func(fact Fact) error {
// 		facts = append(facts, fact)
// 		return nil
// 	})
//
// 	require.NoError(t, err)
// 	for _, f := range facts {
// 		assert.NotEqual(t, facts.RelatedEntityIDKind, f.Kind)
// 		assert.NotEqual(t, facts.EntityKindKind, f.Kind)
// 	}
// }
