package collector

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/url"
	"sort"
	"time"

	"github.com/go-ldap/ldap/v3"

	"github.com/leaktk/leaktk/pkg/config"
)

func parseLDAPScope(scope string) (int, error) {
	switch scope {
	case "base":
		return ldap.ScopeBaseObject, nil
	case "one":
		return ldap.ScopeSingleLevel, nil
	case "sub":
		return ldap.ScopeWholeSubtree, nil
	default:
		return 0, fmt.Errorf("unknown LDAP scope: %q", scope)
	}
}

type attrMapping struct {
	ldapAttr string
	factKind FactKind
}

func resolveAttrMappings(attrMap map[string]string) ([]attrMapping, error) {
	mappings := make([]attrMapping, 0, len(attrMap))
	for ldapAttr, factKindName := range attrMap {
		fk, ok := factKindByName(factKindName)
		if !ok {
			return nil, fmt.Errorf("unknown fact kind in attribute_map: %q", factKindName)
		}
		mappings = append(mappings, attrMapping{ldapAttr: ldapAttr, factKind: fk})
	}

	sort.Slice(mappings, func(i, j int) bool {
		return mappings[i].factKind < mappings[j].factKind
	})

	return mappings, nil
}

// ldapEntryURL builds an RFC 4516 LDAP URL for a specific entry.
func ldapEntryURL(serverURL, dn string) string {
	return serverURL + "/" + url.PathEscape(dn)
}

func yieldLDAPEntryFacts(entries []*ldap.Entry, mappings []attrMapping, sourceID, serverURL string, eidOffset uint32, yield FactYieldFunc) (uint32, error) {
	var err error
	fact := Fact{Timestamp: time.Now().Unix()}

	for _, entry := range entries {
		fact.EntityID = eidOffset
		eidOffset++

		for _, m := range mappings {
			value := entry.GetAttributeValue(m.ldapAttr)
			if len(value) > 0 {
				err = yieldKV(fact, m.factKind, value, yield, err)
			}
		}

		err = yieldKV(fact, SourceIDFactKind, sourceID, yield, err)
		err = yieldKV(fact, URLFactKind, ldapEntryURL(serverURL, entry.DN), yield, err)

		if err != nil {
			return eidOffset, fmt.Errorf("yield LDAP facts: %w", err)
		}
	}

	return eidOffset, nil
}

func ldapDial(rawURL string) (*ldap.Conn, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("parse LDAP URL: %w", err)
	}

	switch u.Scheme {
	case "ldaps":
		return ldap.DialURL(rawURL, ldap.DialWithTLSConfig(&tls.Config{
			ServerName: u.Hostname(),
		}))
	case "ldap":
		return ldap.DialURL(rawURL)
	default:
		return nil, fmt.Errorf("unsupported LDAP URL scheme: %q", u.Scheme)
	}
}

func ldapFacts(_ context.Context, src *config.LDAPSource, eidOffset uint32, yield FactYieldFunc) (uint32, error) {
	mappings, err := resolveAttrMappings(src.AttributeMap)
	if err != nil {
		return eidOffset, err
	}

	attrs := make([]string, 0, len(mappings))
	for _, m := range mappings {
		attrs = append(attrs, m.ldapAttr)
	}

	conn, err := ldapDial(src.URL)
	if err != nil {
		return eidOffset, fmt.Errorf("LDAP dial: %w", err)
	}
	defer func() { _ = conn.Close() }()

	if err = conn.Bind(src.Username, src.Password); err != nil {
		return eidOffset, fmt.Errorf("LDAP bind: %w", err)
	}

	scope, err := parseLDAPScope(src.Scope)
	if err != nil {
		return eidOffset, err
	}

	searchReq := ldap.NewSearchRequest(
		src.BaseDN,
		scope,
		ldap.NeverDerefAliases,
		0,
		0,
		false,
		src.Filter,
		attrs,
		nil,
	)

	result, err := conn.SearchWithPaging(searchReq, 100)
	if err != nil {
		return eidOffset, fmt.Errorf("LDAP search: %w", err)
	}

	return yieldLDAPEntryFacts(result.Entries, mappings, src.ID(), src.URL, eidOffset, yield)
}
