package config

/*
 * IMPORTANT: New fields will need to be mapped in Sources.UnmarshalTOML
 */

type Source interface {
	ID() string
	Kind() SourceKind
	Auth
}

type AtlassianCloudAdminSource struct {
	id    string
	OrgID string
	BearerAuth
}

func (s *AtlassianCloudAdminSource) ID() string {
	return s.id
}

func (s *AtlassianCloudAdminSource) Kind() SourceKind {
	return AtlassianCloudAdminSourceKind
}

type AtlassianCloudJiraSource struct {
	id string
	BasicAuth
}

func (s *AtlassianCloudJiraSource) ID() string {
	return s.id
}

func (s *AtlassianCloudJiraSource) Kind() SourceKind {
	return AtlassianCloudJiraSourceKind
}
