package sources

type LDAP struct {
	id         string
	URL        string
	Username   string
	Password   string
	BaseDN     string
	Filter     string
	Scope      string
	Mapping    facts.Mapping
	Extractors facts.Extractors
}

func (s *LDAP) ID() string {
	return s.id
}

func (s *LDAP) Kind() Kind {
	return LDAPKind
}
