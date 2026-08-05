package patterns

type Resolver struct {
	BaseURL    string
	HTTPClient *http.Client
	CacheDir   string // Defaults to ~/.cache/leaktk
}

func NewResolver(baseURL string) (*Resolver, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("unable to resolve user home dir: %w", err)
	}

	return &Resolver{
		BaseURL:    strings.TrimRight(baseURL, "/"),
		HTTPClient: http.DefaultClient,
		CacheDir:   filepath.Join(home, ".cache", "leaktk"),
	}, nil
}
