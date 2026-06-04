package terminal

import (
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
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

func getAgyConversationIDs(pids []int) map[int]string {
	result := make(map[int]string)
	if len(pids) == 0 {
		return result
	}
	pidStrs := make([]string, len(pids))
	for i, pid := range pids {
		pidStrs[i] = strconv.Itoa(pid)
	}

	cmd := exec.Command("lsof", "-F", "pfn", "-p", strings.Join(pidStrs, ","))
	out, err := cmd.Output()
	if err != nil {
		return result
	}

	re := regexp.MustCompile(`\.gemini/antigravity(?:-cli)?/(?:brain|conversations)/([a-fA-F0-9\-]{36})`)

	currentPID := 0
	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}
		if line[0] == 'p' {
			pid, err := strconv.Atoi(line[1:])
			if err == nil {
				currentPID = pid
			}
		} else if line[0] == 'n' && currentPID != 0 {
			path := line[1:]
			matches := re.FindStringSubmatch(path)
			if len(matches) > 1 {
				result[currentPID] = matches[1]
			}
		}
	}
	return result
}

func getAgyStatus(home string, convID string) (string, string) {
	paths := []string{
		filepath.Join(home, ".gemini", "antigravity-cli", "brain", convID, ".system_generated", "logs", "transcript.jsonl"),
		filepath.Join(home, ".gemini", "antigravity", "brain", convID, ".system_generated", "logs", "transcript.jsonl"),
	}

	var transcriptPath string
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			transcriptPath = p
			break
		}
	}

	if transcriptPath == "" {
		return "idle", ""
	}

	file, err := os.Open(transcriptPath)
	if err != nil {
		return "idle", ""
	}
	defer file.Close()

	stat, err := file.Stat()
	if err != nil {
		return "idle", ""
	}

	var data []byte
	const bufSize = 8192
	if stat.Size() <= bufSize {
		data = make([]byte, stat.Size())
		_, _ = file.Read(data)
	} else {
		data = make([]byte, bufSize)
		_, err = file.ReadAt(data, stat.Size()-bufSize)
		if err != nil && err != io.EOF {
			return "idle", ""
		}
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) == 0 {
		return "idle", ""
	}

	var lastLine string
	for i := len(lines) - 1; i >= 0; i-- {
		trimmed := strings.TrimSpace(lines[i])
		if trimmed != "" {
			lastLine = trimmed
			break
		}
	}

	if lastLine == "" {
		return "idle", ""
	}

	type Step struct {
		Type      string `json:"type"`
		ToolCalls []struct {
			Name string `json:"name"`
		} `json:"tool_calls"`
	}

	var step Step
	if err := json.Unmarshal([]byte(lastLine), &step); err != nil {
		return "idle", ""
	}

	switch step.Type {
	case "USER_INPUT":
		return "busy", ""
	case "PLANNER_RESPONSE":
		if len(step.ToolCalls) == 0 {
			return "idle", ""
		}
		hasApprovalTool := false
		for _, tc := range step.ToolCalls {
			if isApprovalRequired(tc.Name) {
				hasApprovalTool = true
				break
			}
		}
		if hasApprovalTool {
			return "waiting", "approval"
		}
		return "busy", ""
	default:
		return "busy", ""
	}
}

func isApprovalRequired(toolName string) bool {
	switch toolName {
	case "run_command", "write_to_file", "replace_file_content", "multi_replace_file_content", "ask_permission", "ask_question":
		return true
	}
	return false
}


