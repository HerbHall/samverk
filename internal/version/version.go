// Package version provides build-injected version information.
package version

import (
	"regexp"
	"strings"
)

// These variables are set at build time via -ldflags.
var (
	Version   = "dev"
	GitCommit = "unknown"
	BuildDate = "unknown"
)

// CanaryOpus verifies the claude-opus provider path.
const CanaryOpus = "complex-chain"

// ParseGitDescribe extracts the commit SHA from a git describe format version.
// Handles formats like: v0.1.24, v0.1.24-58-ge6ceaf9
// Returns the commit SHA (e.g., "e6ceaf9" or the full hash if only a version).
func ParseGitDescribe(version string) string {
	// Remove leading 'v' if present
	version = strings.TrimPrefix(version, "v")

	// Try to extract from git describe format: v0.1.24-58-ge6ceaf9
	// Pattern: anything-<count>-g<hash>
	re := regexp.MustCompile(`-(\d+)-g([a-f0-9]+)$`)
	matches := re.FindStringSubmatch(version)
	if len(matches) >= 3 {
		return matches[2] // Return the hash part
	}

	// If no match, return the last segment if it looks like a commit hash
	parts := strings.Split(version, "-")
	if len(parts) > 0 {
		last := parts[len(parts)-1]
		// Check if it's a git hash (hex string, 7+ chars)
		if matched, _ := regexp.MatchString(`^[a-f0-9]{7,}$`, last); matched {
			return last
		}
	}

	// Fallback: return as-is if it's already a short hash
	if matched, _ := regexp.MatchString(`^[a-f0-9]{7,}$`, version); matched {
		return version
	}

	return ""
}
