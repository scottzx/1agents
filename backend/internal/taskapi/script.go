package taskapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	scriptCapabilityPrefix = "script:"
	defaultScriptName      = "automation.py"
	defaultScriptTimeout   = 10 * time.Minute
)

func init() {
	RegisterFunction("core.script", runCoreScript)
}

func scriptFromCapabilities(caps []string) string {
	for _, cap := range caps {
		if strings.HasPrefix(cap, scriptCapabilityPrefix) {
			return strings.TrimSpace(strings.TrimPrefix(cap, scriptCapabilityPrefix))
		}
	}
	return ""
}

func resolveScriptPath(cwd, rel string) (string, error) {
	if strings.TrimSpace(cwd) == "" {
		return "", fmt.Errorf("cwd is required")
	}
	if rel == "" {
		rel = defaultScriptName
	}
	if filepath.IsAbs(rel) {
		return "", fmt.Errorf("script path must be relative to cwd")
	}
	clean := filepath.Clean(rel)
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("script path escapes cwd")
	}
	abs := filepath.Join(cwd, clean)
	relToCwd, err := filepath.Rel(cwd, abs)
	if err != nil || relToCwd == ".." || strings.HasPrefix(relToCwd, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("script path escapes cwd")
	}
	return abs, nil
}

func runCoreScript(ctx FunctionContext) (any, error) {
	timeout := ctx.Timeout
	if timeout <= 0 {
		timeout = defaultScriptTimeout
	}
	scriptPath, err := resolveScriptPath(ctx.Cwd, ctx.Script)
	if err != nil {
		return nil, err
	}
	runCtx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(runCtx, "python3", scriptPath)
	cmd.Dir = ctx.Cwd
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if runCtx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("script timed out after %s", timeout)
		}
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("script failed: %s", msg)
	}
	raw := bytes.TrimSpace(stdout.Bytes())
	if len(raw) == 0 {
		return nil, fmt.Errorf("script produced empty stdout; expected JSON")
	}
	var parsed any
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("script stdout is not JSON: %w", err)
	}
	return parsed, nil
}
