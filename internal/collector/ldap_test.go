package collector

import (
	"testing"

	"github.com/go-ldap/ldap/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseLDAPScope(t *testing.T) {
	tests := []struct {
		input    string
		expected int
	}{
		{"base", ldap.ScopeBaseObject},
		{"one", ldap.ScopeSingleLevel},
		{"sub", ldap.ScopeWholeSubtree},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			scope, err := parseLDAPScope(tt.input)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, scope)
		})
	}

	t.Run("invalid", func(t *testing.T) {
		_, err := parseLDAPScope("invalid")
		assert.ErrorContains(t, err, "unknown LDAP scope")
	})
}

func TestFactKindByName(t *testing.T) {
	for i, name := range FactKindNames {
		t.Run(name, func(t *testing.T) {
			fk, ok := factKindByName(name)
			require.True(t, ok)
			assert.Equal(t, FactKind(uint32(i)), fk) // #nosec:G115
		})
	}

	t.Run("unknown", func(t *testing.T) {
		_, ok := factKindByName("DoesNotExist")
		assert.False(t, ok)
	})
}

func TestResolveAttrMappings(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		mappings, err := resolveAttrMappings(map[string]string{
			"cn":   "Name",
			"uuid": "ID",
			"uid":  "Username",
			"mail": "EmailAddress",
		})
		require.NoError(t, err)
		require.Len(t, mappings, 4)

		// Should be sorted by FactKind ascending: ID(0), EmailAddress(2), Name(5), Username(8)
		assert.Equal(t, IDFactKind, mappings[0].factKind)
		assert.Equal(t, "uuid", mappings[0].ldapAttr)
		assert.Equal(t, EmailAddressFactKind, mappings[1].factKind)
		assert.Equal(t, "mail", mappings[1].ldapAttr)
		assert.Equal(t, NameFactKind, mappings[2].factKind)
		assert.Equal(t, "cn", mappings[2].ldapAttr)
		assert.Equal(t, UsernameFactKind, mappings[3].factKind)
		assert.Equal(t, "uid", mappings[3].ldapAttr)
	})

	t.Run("unknown fact kind", func(t *testing.T) {
		_, err := resolveAttrMappings(map[string]string{
			"cn": "DoesNotExist",
		})
		assert.ErrorContains(t, err, "unknown fact kind in attribute_map")
	})

	t.Run("empty", func(t *testing.T) {
		mappings, err := resolveAttrMappings(map[string]string{})
		require.NoError(t, err)
		assert.Empty(t, mappings)
	})
}

func TestLDAPEntryURL(t *testing.T) {
	tests := []struct {
		name      string
		serverURL string
		dn        string
		expected  string
	}{
		{
			name:      "simple",
			serverURL: "ldaps://ldap.example.com:636",
			dn:        "uid=jdoe,ou=people,dc=example,dc=com",
			expected:  "ldaps://ldap.example.com:636/uid=jdoe%2Cou=people%2Cdc=example%2Cdc=com",
		},
		{
			name:      "special characters",
			serverURL: "ldap://ldap.example.com",
			dn:        "cn=John Doe+uid=jdoe,ou=people,dc=example,dc=com",
			expected:  "ldap://ldap.example.com/cn=John%20Doe+uid=jdoe%2Cou=people%2Cdc=example%2Cdc=com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, ldapEntryURL(tt.serverURL, tt.dn))
		})
	}
}

func TestYieldLDAPEntryFacts(t *testing.T) {
	mappings := []attrMapping{
		{ldapAttr: "uuid", factKind: IDFactKind},
		{ldapAttr: "mail", factKind: EmailAddressFactKind},
		{ldapAttr: "cn", factKind: NameFactKind},
		{ldapAttr: "uid", factKind: UsernameFactKind},
	}

	entries := []*ldap.Entry{
		ldap.NewEntry("uid=jdoe,ou=people,dc=example,dc=com", map[string][]string{
			"uuid": {"550e8400-e29b-41d4-a716-446655440000"},
			"uid":  {"jdoe"},
			"mail": {"jdoe@example.com"},
			"cn":   {"John Doe"},
		}),
		ldap.NewEntry("uid=asmith,ou=people,dc=example,dc=com", map[string][]string{
			"uuid": {"550e8400-e29b-41d4-a716-446655440001"},
			"uid":  {"asmith"},
			"cn":   {"Alice Smith"},
		}),
	}

	var facts []Fact
	eidOffset, err := yieldLDAPEntryFacts(entries, mappings, "test-ldap", "ldaps://ldap.example.com:636", 1, func(fact Fact) error {
		facts = append(facts, fact)
		return nil
	})

	require.NoError(t, err)
	assert.Equal(t, uint32(3), eidOffset)

	actual := make(map[uint32]map[string]string)
	for _, fact := range facts {
		emap, ok := actual[fact.EntityID]
		if !ok {
			emap = make(map[string]string)
			actual[fact.EntityID] = emap
		}
		emap[fact.Kind.String()] = fact.Value
	}

	assert.Equal(t, map[uint32]map[string]string{
		1: {
			IDFactKind.String():           "550e8400-e29b-41d4-a716-446655440000",
			EmailAddressFactKind.String(): "jdoe@example.com",
			NameFactKind.String():         "John Doe",
			SourceIDFactKind.String():     "test-ldap",
			URLFactKind.String():          "ldaps://ldap.example.com:636/uid=jdoe%2Cou=people%2Cdc=example%2Cdc=com",
			UsernameFactKind.String():     "jdoe",
		},
		2: {
			IDFactKind.String():       "550e8400-e29b-41d4-a716-446655440001",
			NameFactKind.String():     "Alice Smith",
			SourceIDFactKind.String(): "test-ldap",
			URLFactKind.String():      "ldaps://ldap.example.com:636/uid=asmith%2Cou=people%2Cdc=example%2Cdc=com",
			UsernameFactKind.String(): "asmith",
		},
	}, actual)
}

func TestYieldLDAPEntryFactsOrdering(t *testing.T) {
	mappings := []attrMapping{
		{ldapAttr: "uuid", factKind: IDFactKind},
		{ldapAttr: "mail", factKind: EmailAddressFactKind},
		{ldapAttr: "cn", factKind: NameFactKind},
		{ldapAttr: "uid", factKind: UsernameFactKind},
	}

	entries := []*ldap.Entry{
		ldap.NewEntry("uid=jdoe,ou=people,dc=example,dc=com", map[string][]string{
			"uuid": {"550e8400-e29b-41d4-a716-446655440000"},
			"uid":  {"jdoe"},
			"mail": {"jdoe@example.com"},
			"cn":   {"John Doe"},
		}),
	}

	var kinds []FactKind
	_, err := yieldLDAPEntryFacts(entries, mappings, "test-ldap", "ldaps://ldap.example.com:636", 1, func(fact Fact) error {
		kinds = append(kinds, fact.Kind)
		return nil
	})

	require.NoError(t, err)

	// Mapped facts in ascending FactKind order, then implicit facts (SourceID, URL)
	assert.Equal(t, []FactKind{
		IDFactKind,
		EmailAddressFactKind,
		NameFactKind,
		UsernameFactKind,
		SourceIDFactKind,
		URLFactKind,
	}, kinds)
}

func TestYieldLDAPEntryFactsEmpty(t *testing.T) {
	mappings := []attrMapping{
		{ldapAttr: "uid", factKind: IDFactKind},
	}

	eidOffset, err := yieldLDAPEntryFacts(nil, mappings, "test-ldap", "ldaps://ldap.example.com:636", 5, func(fact Fact) error {
		t.Fatal("yield should not be called for empty entries")
		return nil
	})

	require.NoError(t, err)
	assert.Equal(t, uint32(5), eidOffset)
}
