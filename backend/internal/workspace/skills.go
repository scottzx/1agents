package workspace

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// newAssistantBadge mints an assistant identity badge. It remains workspace
// identity logic; no extension-store behavior lives in this file.
func newAssistantBadge() string {
	date := time.Now().Format("20060102")
	prefix := date + "-"
	root := filepath.Join(get1AgentsHome(), ".1agents", "projects")
	max := 0
	if entries, err := os.ReadDir(root); err == nil {
		for _, entry := range entries {
			if !entry.IsDir() || !strings.HasPrefix(entry.Name(), prefix) {
				continue
			}
			if value, err := strconv.Atoi(entry.Name()[len(prefix):]); err == nil && value > max {
				max = value
			}
		}
	}
	return fmt.Sprintf("%s-%04d", date, max+1)
}
