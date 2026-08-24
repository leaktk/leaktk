package healthcheck

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func checkGitIgnoreEnv(project string, fix bool) (*Finding, error) {
	path := filepath.Join(project, ".gitignore")
	ignored, err := ignoresEnv(path)
	if err != nil {
		return nil, err
	}
	if ignored {
		return nil, nil
	}

	finding := &Finding{
		Policy:      gitIgnoreEnvPolicy,
		Path:        path,
		Summary:     ".env is not ignored by .gitignore",
		Remediation: "add .env to .gitignore",
	}
	if !fix {
		return finding, nil
	}

	if err := addEnvIgnore(path); err != nil {
		return nil, err
	}
	finding.Fixed = true

	return finding, nil
}

func ignoresEnv(path string) (bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}

		return false, fmt.Errorf("could not read .gitignore: %w path=%q", err, path)
	}

	ignored := false
	for _, line := range strings.Split(string(data), "\n") {
		pattern, negated := gitIgnorePattern(line)
		if (!(pattern == ".env" || pattern == "/.env" || pattern == "**/.env" || pattern == "*.env")) {
			continue
		}

		ignored = !negated
	}

	return ignored, nil
}

func gitIgnorePattern(line string) (string, bool) {
	pattern := strings.TrimSuffix(line, "\r")
	if len(pattern) == 0 || strings.HasPrefix(pattern, "#") {
		return "", false
	}

	negated := strings.HasPrefix(pattern, "!")
	if negated {
		pattern = strings.TrimPrefix(pattern, "!")
	}

	return pattern, negated
}

func addEnvIgnore(path string) error {
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("could not read .gitignore: %w path=%q", err, path)
	}

	lineEnding := "\n"
	if bytes.Contains(data, []byte("\r\n")) {
		lineEnding = "\r\n"
	}
	if len(data) > 0 && !bytes.HasSuffix(data, []byte("\n")) {
		data = append(data, lineEnding...)
	}
	data = append(data, ".env"...)
	data = append(data, lineEnding...)

	mode := os.FileMode(0600)
	if info, statErr := os.Stat(path); statErr == nil {
		mode = info.Mode().Perm()
	}
	if err := os.WriteFile(path, data, mode); err != nil {
		return fmt.Errorf("could not update .gitignore: %w path=%q", err, path)
	}

	return nil
}
