package git

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math/bits"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/leaktk/leaktk/pkg/config"
	"github.com/leaktk/leaktk/pkg/logger"
)

type VersionInfo struct {
	X int
	Y int
	Z int
}

func (v VersionInfo) String() string {
	return fmt.Sprintf("%d.%d.%d", v.X, v.Y, v.Z)
}

func (v VersionInfo) Compare(o *VersionInfo) int {
	x := v.X - o.X
	y := v.Y - o.Y
	z := v.Z - o.Z
	shift := bits.UintSize - 1

	if x != 0 {
		return int(uint(-x)>>shift) - int(uint(x)>>shift) //#nosec G115
	}
	if y != 0 {
		return int(uint(-y)>>shift) - int(uint(y)>>shift) //#nosec G115
	}
	if z != 0 {
		return int(uint(-z)>>shift) - int(uint(z)>>shift) //#nosec G115
	}

	return 0
}

// GitRepoInfo is a collection of facts about a repo being scanned.
// See `man 7 gitglossary` for more information about the terms.
type RepoInfo struct {
	// Whether or not the repo is a bare repo
	IsBare bool
	// The path to the actual GIT_DIR folder
	GitDir string
	// The working tree for the repo (a temp one is created for bare repos)
	WorkingTreePath string
}

var versionRe = regexp.MustCompile(`^git\sversion\s(\d+)\.(\d+)\.(\d+)`)

func GetRepoInfo(ctx context.Context, path string) (RepoInfo, error) {
	info := RepoInfo{WorkingTreePath: path}
	cmd := CommandContext(
		ctx,
		"-C",
		path,
		"rev-parse",
		// The order of these flags affects the field order below
		"--absolute-git-dir",
		"--is-bare-repository",
	) // #nosec G204

	logger.Debug("executing: %s", cmd)
	rawInfo, err := cmd.Output()
	if err != nil {
		return info, err
	}

	fields := bytes.Split(bytes.TrimSpace(rawInfo), []byte("\n"))
	if len(fields) != 2 {
		return info, errors.New("could not load git repo info")
	}

	// Load the field data from above
	info.GitDir = string(fields[0])
	info.IsBare = bytes.Equal(fields[1], []byte("true"))

	// Resolve the working tree to the toplevel path
	if !info.IsBare {
		// Running this separate since it's more prone to error out
		cmd := CommandContext(
			ctx,
			"-C",
			info.WorkingTreePath,
			"rev-parse",
			"--show-toplevel",
		) // #nosec G204
		logger.Debug("executing: %s", cmd)
		rawTopLevel, err := cmd.Output()
		if err == nil {
			info.WorkingTreePath = string(bytes.TrimSpace(rawTopLevel))
			logger.Debug("setting working tree to toplevel dir: path=%q", info.WorkingTreePath)
		} else {
			logger.Debug("unable to set working tree: %v", err)
		}
	}

	return info, nil
}

func RunContext(ctx context.Context, args ...string) error {
	cmd := CommandContext(ctx, args...)
	logger.Debug("executing: %s", cmd)
	return cmd.Run()
}

// RemoteRefExists checks if the provided ref exists on the remote repo
func RemoteRefExists(ctx context.Context, repository, ref string) bool {
	return RunContext(ctx, "ls-remote", "--exit-code", "--quiet", repository, ref) == nil
}

// GetConfig gets a value from the config and applies provided flags
func GetConfig(ctx context.Context, name string, flags ...string) string {
	// Handle undoing the override for this operation
	if os.Getenv("GIT_CONFIG_GLOBAL") == config.GitConfigGlobalOverride {
		if err := os.Unsetenv("GIT_CONFIG_GLOBAL"); err != nil {
			logger.Fatal("Unable to properly configure git env: %v", err)
		}
	}

	logger.Debug("getting global config value: name=%q flags=%q", name, strings.Join(flags, " "))
	args := make([]string, 0, len(flags)+2)
	args = append(args, "config")
	args = append(args, flags...)
	args = append(args, name)
	cmd := CommandContext(ctx, args...)

	logger.Debug("executing: %s", cmd)
	output, err := cmd.Output()
	if err != nil {
		// if the value isn't set properly or at all, treat it like it's not set at all
		logger.Debug("existing value not found: %v name=%q", err, name)
		if err := os.Setenv("GIT_CONFIG_GLOBAL", config.GitConfigGlobalOverride); err != nil {
			logger.Fatal("Unable to properly configure git env: %v", err)
		}

		return ""
	}

	if err := os.Setenv("GIT_CONFIG_GLOBAL", config.GitConfigGlobalOverride); err != nil {
		logger.Fatal("Unable to properly configure git env: %v", err)
	}
	return strings.TrimSpace(string(output))
}

// SetConfig sets a value in the config
func SetConfig(ctx context.Context, name, value string, flags ...string) error {
	// Handle undoing the override for this operation
	if os.Getenv("GIT_CONFIG_GLOBAL") == config.GitConfigGlobalOverride {
		if err := os.Unsetenv("GIT_CONFIG_GLOBAL"); err != nil {
			logger.Fatal("Unable to properly configure git env: %v", err)
		}
	}

	logger.Debug("setting config value: name=%q value=%q flags=%q", name, value, strings.Join(flags, " "))

	args := make([]string, 0, len(flags)+3)
	args = append(args, "config")
	args = append(args, flags...)
	args = append(args, name)
	args = append(args, value)

	if err := RunContext(ctx, args...); err != nil {
		if err := os.Setenv("GIT_CONFIG_GLOBAL", config.GitConfigGlobalOverride); err != nil {
			logger.Fatal("Unable to properly configure git env: %v", err)
		}
		return fmt.Errorf("could not set git config value: %w name=%q value=%q", err, name, value)
	}

	if err := os.Setenv("GIT_CONFIG_GLOBAL", config.GitConfigGlobalOverride); err != nil {
		logger.Fatal("Unable to properly configure git env: %v", err)
	}
	return nil
}

func Version(ctx context.Context) VersionInfo {
	v := VersionInfo{}
	cmd := CommandContext(ctx, "version")
	logger.Debug("executing: %s", cmd)
	rawVersion, err := cmd.Output()
	if err != nil {
		logger.Error("could not get git version: %v", err)
		return v
	}

	m := versionRe.FindSubmatch(rawVersion)
	if len(m) != 0 {
		v.X, _ = strconv.Atoi(string(m[1]))
		v.Y, _ = strconv.Atoi(string(m[2]))
		v.Z, _ = strconv.Atoi(string(m[3]))
	}

	return v
}
