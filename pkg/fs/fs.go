package fs

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// FileExists checks to see if a path exists and is a file
func FileExists(path string) bool {
	info, err := os.Stat(path)

	if err != nil && !os.IsNotExist(err) {
		return false
	}

	return info != nil && err == nil && !info.IsDir()
}

// DirExists checks to see if a path exists and is a dir
func DirExists(path string) bool {
	info, err := os.Stat(path)

	if err != nil && !os.IsNotExist(err) {
		return false
	}

	return info != nil && err == nil && info.IsDir()
}

// PathExists checks to see if a path exists
func PathExists(path string) bool {
	info, err := os.Stat(path)

	if err != nil && !os.IsNotExist(err) {
		return false
	}

	return info != nil && err == nil
}

// CleanJoin checks to make sure that the prefix path remains after the join, this is to
// control for path traversal
func CleanJoin(prefix string, elem string) (string, error) {
	destPath := filepath.Join(prefix, elem)
	if !strings.HasPrefix(destPath, filepath.Clean(prefix)) {
		return "", fmt.Errorf("illegal file path: %s", elem)
	}

	return destPath, nil
}

// Split splits a file path into its parts
func Split(path string) []string {
	prefix, part := filepath.Split(path)

	if prefix == "" {
		return []string{part}
	}

	return append(Split(filepath.Clean(prefix)), part)
}

// Match does basic glob matching. It is similar to filepath.Match except
// it currently only supports wildcards (*) and recursive wildcards (**), which
// is not supported by filepath.Match
func Match(pattern, path string) bool {
	patternParts := Split(pattern)
	pathParts := Split(path)

	return matchParts(patternParts, pathParts)
}

func matchParts(patternParts, pathParts []string) bool {
	pIdx, ptIdx := 0, 0

	for pIdx < len(patternParts) && ptIdx < len(pathParts) {
		switch patternParts[pIdx] {
		case "*": // Match a single segment
			// Move to next part of both the pattern and path
			pIdx++
			ptIdx++
		case "**": // Match zero or more segments
			// If this is the last pattern part, we match the rest of the path
			if pIdx == len(patternParts)-1 {
				return true
			}
			// Try matching subsequent parts
			for i := ptIdx; i <= len(pathParts); i++ {
				if matchParts(patternParts[pIdx+1:], pathParts[i:]) {
					return true
				}
			}

			return false
		default:
			// Exact match required for this segment
			if patternParts[pIdx] != pathParts[ptIdx] {
				return false
			}
			pIdx++
			ptIdx++
		}
	}

	// Both pattern and path should be fully consumed for a match
	return pIdx == len(patternParts) && ptIdx == len(pathParts)
}
