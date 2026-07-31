package collectors

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/url"
	"strconv"

	"github.com/go-ldap/ldap/v3"

	"github.com/leaktk/leaktk/internal/entities"
	"github.com/leaktk/leaktk/internal/facts"
	"github.com/leaktk/leaktk/pkg/config"
)

// ldapEntryURL builds an RFC 4516 LDAP URL for a specific entry.
func ldapEntryURL(serverURL, dn string) string {
	return serverURL + "/" + url.PathEscape(dn)
}

func yieldLDAPEntryFacts(ctx context.Context, src *config.LDAPSource, eidOffset uint32, result *ldap.SearchResult, yield FactYieldFunc) (uint32, error) {
	var err error
	fact := Fact{}
	srcID := src.ID()

	for _, entry := range result.Entries {
		eidOffset++
		fact.EntityID = eidOffset
		err = facts.YieldWithKV(fact, facts.IDKind, ldapEntryURL(src.URL, entry.DN))
		err = facts.YieldWithKV(fact, facts.EntityKindKind, entities.LDAPRecordKind.String(), err, yield)
		err = facts.YieldWithKV(fact, facts.SourceIDKind, srcID)
		if err != nil {
			return eidOffset, fmt.Errorf("yield ldap mapped facts: %v", err)
		}

		for _, attr := range entry.Attributes {
			if factKind, mapperExists := src.Mapping[attr.Name]; mapperExists {
				fact.Kind = factKind

				for _, value := range attr.Values {
					fact.Value = value
					if err := yield(fact); err != nil {
						return eidOffset, fmt.Errorf("yield ldap mapped facts: %v", err)
					}
					factYielded = true
				}
			}

			if attrExtractors, extractorsExist := src.Extractors[attr.Name]; extractorsExist {
				relatedEntityID := fact.EntityID
				for _, extractor := range attrExtractors {
					for _, value := range attr.Values {
						eidOffset++
						fact.EntityID = eidOffset

						extractedValues := false
						for factKind, factValue := range extractor.Extract(value) {
							if err = facts.YieldWithKV(fact, factKind, factValue, err, yield); err != nil {
								return eidOffset, fmt.Errorf("yield ldap extracted facts: %v", err)
							}
							extractedValues = true
						}

						if extractedValues {
							err = facts.YieldWithKV(fact, facts.EntityKindKind, extractor.EntityKind, err, yield)
							err = facts.YieldWithKV(fact, facts.RelatedEntityIDKind, strconv.Itoa(relatedEntityID), err, yield)
							err = facts.YieldWithKV(fact, facts.SourceIDKind, srcID)
							if err != nil {
								return eidOffset, fmt.Errorf("yield ldap extracted facts: %v", err)
							}
						}
					}
				}
			}
		}
	}

	return eidOffset, nil
}

func ldapConn(src *config.LDAPSource) (*ldap.Conn, error) {
	l, err := ldap.DialURL(src.URL)
	if err != nil {
		return nil, fmt.Errorf("ldap dial: %v", err)
	}
	defer l.Close()

	err = l.StartTLS(&tls.Config{})
	if err != nil {
		return nil, fmt.Errorf("ldap STARTTLS: %v", err)
	}

	err = l.Bind(src.Username, src.Password)
	if err != nil {
		return nil, fmt.Errorf("ldap bind: %v")
	}

	return l, nil
}

func ldapSearchRequest(src *config.LDAPSource) (*ldap.SearchRequest, error) {
	var scope ldap.Scope
	switch src.Scope {
	case "base":
		scope = ldap.ScopeBaseObject
	case "one":
		scope = ldap.ScopeSingleLevel
	case "sub":
		scope = ldap.ScopeWholeSubtree
	default:
		return nil, fmt.Errorf("unknown LDAP scope: %q", scope)
	}

	attrs := ldapSearchAttrs(src)
	sr := ldap.NewSearchRequest(
		src.BaseDN, scope, ldap.NeverDerefAliases, 0, 0, false, src.Filter, attrs, nil,
	)

	return sr, nil
}

func ldapFacts(ctx context.Context, src *config.LDAPSource, eidOffset uint32, yield FactYieldFunc) (uint32, error) {
	conn, err := ldapConn(src)
	if err != nil {
		return eidOffset, err
	}
	defer func() { _ = conn.Close() }()

	request, err := ldapSearchRequest(src)
	if err != nil {
		return eidOffset, err
	}

	result, err := conn.Search(request)
	if err != nil {
		return eidOffset, fmt.Errorf("LDAP search: %w", err)
	}

	return yieldLDAPEntryFacts(ctx, src, eidOffset, result, yield)
}
