package healthcheck

import (
	"fmt"
	"os"
	"path/filepath"
)

const gitIgnoreEnvPolicy = "gitignore.env"


type Finding struct {
	Policy      string `json:"policy"`
	Path        string `json:"path"`
	Summary     string `json:"summary"`
	Remediation string `json:"remediation"`
	Fixed       bool   `json:"fixed"`
}

type Result struct {
	Project  string    `json:"project"`
	Findings []Finding `json:"findings"`
}

func (r Result) NeedsAction() bool {
	for _, finding := range r.Findings {
		if !finding.Fixed {
			return true
		}
	}

	return false
}

func Run(path string, fix bool) (*Result, error) {
	project, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("could not resolve project path: %w", err)
	}

	info, err := os.Stat(project)
	if err != nil {
		return nil, fmt.Errorf("couldn't access project: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("project path is not a directory: path=%q", project)
	}

	result := &Result{Project: project}
	if finding, err := checkGitIgnoreEnv(project, fix); err != nil {
		return nil, err
	} else if finding != nil {
		result.Findings = append(result.Findings, *finding)
	}

	return result, nil
}
