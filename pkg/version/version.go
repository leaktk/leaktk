package version

import (
	"fmt"
	"runtime"
)

// Version number set by the build
var Version = ""

// Commit id set by the build
var Commit = ""

// GlobalUserAgent the useragent used by our http requests
var GlobalUserAgent = fmt.Sprintf("leaktk/%s (%s %s)", shortVersion(), runtime.GOOS, runtime.GOARCH)

// PrintVersion prints the version details to stdout
func PrintVersion() {
	if len(Version) > 0 {
		fmt.Printf("Version: %v\n", Version)

		if len(Commit) > 0 {
			fmt.Printf("Commit: %v\n", Commit)
		}
	} else {
		fmt.Println("Version information not available")
	}
}

func shortVersion() string {
	if len(Version) > 0 {
		if len(Commit) > 0 {
			return Version + "@" + Commit
		}

		return Version
	}

	return "unknown"
}
