package roundtable

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// SeatPromptRequest is one model turn against a seat's Grok Build session.
type SeatPromptRequest struct {
	SessionID      string // 1agents chat session id
	WorkspacePath  string
	AgentType      string
	AcpSessionID   string // empty = new; non-empty = 1acp resume (same session multi-turn)
	Text           string
	SystemContext  string // only applied on fresh sessions (no acp id yet)
	PermissionMode string
	// Role is optional metadata for tests / isolation logs (not sent to bridge).
	Role Role
}

// SeatPromptResult is the agent reply for one turn.
type SeatPromptResult struct {
	Text         string
	AcpSessionID string
}

// SeatPrompter runs one continuous-prompt turn on a seat session (design §5.2 R1).
// Injected in tests; production uses BridgeSeatPrompter.
type SeatPrompter interface {
	Prompt(req SeatPromptRequest) (SeatPromptResult, error)
}

// DefaultBridgePort matches server acpxBridgePort (ACPX_PORT or 38082).
func DefaultBridgePort() int {
	if v := os.Getenv("ACPX_PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil && p > 0 {
			return p
		}
	}
	return 38082
}

// BridgeSeatPrompter dials the 1acp bridge-server (same path as agent.TaskRunner)
// and issues ensure_session + prompt, collecting content_text from text_delta.
type BridgeSeatPrompter struct {
	Port        int
	IdleTimeout time.Duration
}

// NewBridgeSeatPrompter builds a production prompter against the local bridge.
func NewBridgeSeatPrompter(port int) *BridgeSeatPrompter {
	if port <= 0 {
		port = DefaultBridgePort()
	}
	return &BridgeSeatPrompter{
		Port:        port,
		IdleTimeout: 5 * time.Minute,
	}
}

// bridgeMsg mirrors agent.WsMessage fields needed for ensure/prompt/done.
type bridgeMsg struct {
	Action          string `json:"action,omitempty"`
	Event           string `json:"event,omitempty"`
	SessionID       string `json:"sessionId,omitempty"`
	WorkspacePath   string `json:"workspacePath,omitempty"`
	AgentType       string `json:"agentType,omitempty"`
	AcpSessionID    string `json:"acpSessionId,omitempty"`
	ResumeSessionID string `json:"resumeSessionId,omitempty"`
	SystemContext   string `json:"systemContext,omitempty"`
	Text            string `json:"text,omitempty"`
	AgentSessionID  string `json:"agentSessionId,omitempty"`
	Message         string `json:"message,omitempty"`
	Type            string `json:"type,omitempty"`
	PermissionMode  string `json:"permissionMode,omitempty"`
	Summary         string `json:"summary,omitempty"`
}

// Prompt starts or resumes a real Grok Build (or other) ACP session and returns
// the assistant's final text block for this turn.
func (p *BridgeSeatPrompter) Prompt(req SeatPromptRequest) (SeatPromptResult, error) {
	if strings.TrimSpace(req.Text) == "" {
		return SeatPromptResult{}, fmt.Errorf("roundtable: empty prompt text")
	}
	if strings.TrimSpace(req.SessionID) == "" || strings.TrimSpace(req.WorkspacePath) == "" {
		return SeatPromptResult{}, fmt.Errorf("roundtable: session_id and workspace path required")
	}
	agentType := req.AgentType
	if agentType == "" {
		agentType = "grok-build"
	}
	mode := req.PermissionMode
	if mode == "" {
		// Pure-chat tmp seats; unattended approvals so R1 does not stall on tools.
		mode = "approve-all"
	}
	timeout := p.IdleTimeout
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}

	serverURL := fmt.Sprintf("ws://127.0.0.1:%d", p.Port)
	conn, _, err := websocket.DefaultDialer.Dial(serverURL, nil)
	if err != nil {
		return SeatPromptResult{}, fmt.Errorf("bridge unavailable: %w", err)
	}
	defer conn.Close()

	// Grok Build does not honor native systemPrompt meta; system context is
	// merged into the first user prompt when acp_session_id is empty (same
	// strategy as agent.AcpxClient pendingSystemContext).
	promptText := req.Text
	systemCtx := ""
	if req.AcpSessionID == "" && strings.TrimSpace(req.SystemContext) != "" {
		// For non-native agents the bridge path merges via pendingSystemContext
		// only when the client is agent.AcpxClient. Headless dial uses ensure_session
		// SystemContext for native agents; for grok-build we prefix the first prompt.
		promptText = strings.TrimSpace(req.SystemContext) + "\n\n---\n\n" + req.Text
	} else if req.AcpSessionID == "" {
		systemCtx = req.SystemContext
	}

	ensure := bridgeMsg{
		Action:          "ensure_session",
		SessionID:       req.SessionID,
		WorkspacePath:   req.WorkspacePath,
		AgentType:       agentType,
		AcpSessionID:    req.AcpSessionID,
		ResumeSessionID: req.AcpSessionID,
		SystemContext:   systemCtx,
		PermissionMode:  mode,
	}
	if err := conn.WriteJSON(ensure); err != nil {
		return SeatPromptResult{}, fmt.Errorf("ensure_session: %w", err)
	}

	acpID := req.AcpSessionID
	var textParts []string
	promptSent := false

	for {
		_ = conn.SetReadDeadline(time.Now().Add(timeout))
		var msg bridgeMsg
		if err := conn.ReadJSON(&msg); err != nil {
			return SeatPromptResult{}, fmt.Errorf("bridge read: %w", err)
		}
		switch msg.Event {
		case "session_ready":
			if msg.AgentSessionID != "" {
				acpID = msg.AgentSessionID
			}
			if !promptSent {
				promptSent = true
				if err := conn.WriteJSON(bridgeMsg{
					Action:    "prompt",
					SessionID: req.SessionID,
					Text:      promptText,
				}); err != nil {
					return SeatPromptResult{}, fmt.Errorf("prompt: %w", err)
				}
			}
		case "text_delta":
			if msg.Type != "thought" && msg.Text != "" {
				textParts = append(textParts, msg.Text)
			}
		case "tool_call":
			// Process noise is not content_text; reset so final text is post-tool.
			textParts = nil
		case "done":
			text := strings.Join(textParts, "")
			if strings.TrimSpace(text) == "" && msg.Summary != "" {
				text = msg.Summary
			}
			// Keep session alive for next R1 turn (do not close_session).
			log.Printf("[roundtable] seat prompt done session=%s acp=%s text_len=%d",
				req.SessionID, acpID, len(text))
			return SeatPromptResult{Text: text, AcpSessionID: acpID}, nil
		case "error":
			return SeatPromptResult{}, fmt.Errorf("agent error: %s", msg.Message)
		}
	}
	return SeatPromptResult{}, fmt.Errorf("agent stream ended unexpectedly")
}

// StaticSeatPrompter is a test double that returns canned replies.
// Safe for concurrent Prompt calls (R2 parallel seats).
type StaticSeatPrompter struct {
	mu sync.Mutex
	// Reply is used when Replies / ReplyByRole / ReplyFunc are empty.
	Reply string
	// Replies are consumed in order for multi-turn tests.
	Replies []string
	// ReplyByRole returns role-specific speech (R2 isolation tests).
	ReplyByRole map[Role]string
	// ReplyFunc overrides canned replies when set.
	ReplyFunc func(req SeatPromptRequest) (string, error)
	// Calls records every Prompt invocation (order not guaranteed under parallel).
	Calls []SeatPromptRequest
	// FailNext makes the next Prompt return an error.
	FailNext error
	// FailRoles makes Prompt fail for those roles (partial R2 failure tests).
	FailRoles map[Role]error
	// FixedAcpID is returned as AcpSessionID (default "acp-test-1").
	FixedAcpID string
	idx        int
}

// Prompt implements SeatPrompter.
func (s *StaticSeatPrompter) Prompt(req SeatPromptRequest) (SeatPromptResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Calls = append(s.Calls, req)
	if s.FailNext != nil {
		err := s.FailNext
		s.FailNext = nil
		return SeatPromptResult{}, err
	}
	if s.FailRoles != nil {
		if err, ok := s.FailRoles[req.Role]; ok && err != nil {
			return SeatPromptResult{}, err
		}
	}
	acp := req.AcpSessionID
	if acp == "" {
		acp = s.FixedAcpID
	}
	if acp == "" {
		if req.Role != "" {
			acp = fmt.Sprintf("acp-%s", RoleSlug(req.Role))
		} else {
			acp = "acp-test-1"
		}
	}
	if s.ReplyFunc != nil {
		text, err := s.ReplyFunc(req)
		if err != nil {
			return SeatPromptResult{}, err
		}
		return SeatPromptResult{Text: text, AcpSessionID: acp}, nil
	}
	text := s.Reply
	if s.ReplyByRole != nil {
		if byRole, ok := s.ReplyByRole[req.Role]; ok && byRole != "" {
			text = byRole
		}
	}
	if len(s.Replies) > 0 {
		if s.idx < len(s.Replies) {
			text = s.Replies[s.idx]
			s.idx++
		} else if text == "" {
			text = s.Replies[len(s.Replies)-1]
		}
	}
	if text == "" {
		if req.Role != "" && req.Role != RoleReferee {
			text = fmt.Sprintf("%s席观点：围绕 Brief 的首轮发言。", RoleLabel(req.Role))
		} else {
			text = "裁判：已收到，请继续补充约束与成功标准。"
		}
	}
	return SeatPromptResult{Text: text, AcpSessionID: acp}, nil
}

// SnapshotCalls returns a copy of recorded calls (safe under concurrent Prompt).
func (s *StaticSeatPrompter) SnapshotCalls() []SeatPromptRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]SeatPromptRequest, len(s.Calls))
	copy(out, s.Calls)
	return out
}
