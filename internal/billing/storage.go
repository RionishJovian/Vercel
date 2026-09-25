package billing

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// SaveReceipt writes a guest receipt photo into a TENANT-SCOPED directory
// (<root>/<businessID>/) and returns its path. The file is created 0600 and its
// directory 0700 — never world-readable. Access is by request id via the repo
// row, not by listing the filesystem.
func SaveReceipt(root, businessID, requestID string, data []byte) (string, error) {
	if root == "" {
		return "", fmt.Errorf("billing: receipt root not configured")
	}
	if businessID == "" || requestID == "" {
		return "", fmt.Errorf("billing: businessID and requestID are required")
	}
	if strings.ContainsAny(businessID, `/\`) || strings.ContainsAny(requestID, `/\`) {
		return "", fmt.Errorf("billing: invalid id")
	}
	if !safeComponent(businessID) || !safeComponent(requestID) {
		return "", fmt.Errorf("billing: unsafe id")
	}

	dir := filepath.Join(root, businessID)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("billing: mkdir receipt dir: %w", err)
	}
	path := filepath.Join(dir, requestID+".bin")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return "", fmt.Errorf("billing: write receipt: %w", err)
	}
	return path, nil
}

// safeComponent rejects empty, dot, dot-dot, and any id that could escape the
// tenant directory once joined into a path.
func safeComponent(s string) bool {
	if s == "" || s == "." || s == ".." {
		return false
	}
	return !strings.Contains(s, "..")
}
