package terminal

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// ClaudeSession represents a Claude Code session details.
type ClaudeSession struct {
	PID        int    `json:"pid"`
	Agent      string `json:"agent"`
	Status     string `json:"status"`
	WaitingFor string `json:"waitingFor"`
}

func getClaudeSessions() ([]ClaudeSession, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	sessionsDir := filepath.Join(home, ".claude", "sessions")
	files, err := os.ReadDir(sessionsDir)
	if err != nil {
		// If directory does not exist, return empty list
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var sessions []ClaudeSession
	for _, f := range files {
		if f.IsDir() || !strings.HasSuffix(f.Name(), ".json") {
			continue
		}
		path := filepath.Join(sessionsDir, f.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var s ClaudeSession
		if err := json.Unmarshal(data, &s); err == nil {
			sessions = append(sessions, s)
		}
	}
	return sessions, nil
}

func getProcessDetails() (map[int]int, map[int]string, error) {
	cmd := exec.Command("ps", "-ax", "-o", "pid,ppid,command")
	out, err := cmd.Output()
	if err != nil {
		return nil, nil, err
	}
	parentMap := make(map[int]int)
	cmdMap := make(map[int]string)
	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "PID") {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) >= 3 {
			pid, err1 := strconv.Atoi(parts[0])
			ppid, err2 := strconv.Atoi(parts[1])
			if err1 == nil && err2 == nil {
				parentMap[pid] = ppid
				cmdMap[pid] = strings.Join(parts[2:], " ")
			}
		}
	}
	return parentMap, cmdMap, nil
}

func isAncestor(parentMap map[int]int, child int, ancestor int) bool {
	current := child
	for {
		parent, ok := parentMap[current]
		if !ok || parent == 0 {
			break
		}
		if parent == ancestor {
			return true
		}
		current = parent
	}
	return false
}




