package roundtable

import (
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/scottzx/1Agents/backend/internal/agent"
	"github.com/scottzx/1Agents/backend/internal/meta"
)

// Service orchestrates room creation, R1/R2/R3 turns, and state changes.
type Service struct {
	store *Store
	// chatStore indexes real Grok Build ChatSessionRecords for seats.
	chatStore *agent.Store
	// createSeatWS is injected for tests; defaults to agent.CreateAppWorkspace
	// (kind=app so sidebar 任务区 excludes these seats).
	createSeatWS func(idSuffix, displayName string) (workspaceID, cwd string, err error)
	// writeSeed is injected for tests; defaults to WriteRoleSeed.
	writeSeed func(cwd string, role Role) error
	// resolveCwd maps workspace_id → absolute path; defaults to meta project lookup.
	resolveCwd func(workspaceID string) (string, error)
	// prompter runs seat model turns; defaults to BridgeSeatPrompter.
	prompter SeatPrompter
}

// NewService builds a Service with the given store and optional chat index.
// chatStore may be nil only in unit tests that never start sessions; production
// always passes a real SessionStore so referee seats get real Grok Build sessions.
func NewService(store *Store, chatStore *agent.Store, prompter SeatPrompter) *Service {
	if prompter == nil {
		prompter = NewBridgeSeatPrompter(DefaultBridgePort())
	}
	return &Service{
		store:        store,
		chatStore:    chatStore,
		createSeatWS: agent.CreateAppWorkspace,
		writeSeed:    WriteRoleSeed,
		resolveCwd:   defaultResolveCwd,
		prompter:     prompter,
	}
}

func defaultResolveCwd(workspaceID string) (string, error) {
	db, err := meta.OpenDefault()
	if err != nil {
		return "", err
	}
	proj, ok, err := db.GetProject(workspaceID)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", fmt.Errorf("workspace %s not found", workspaceID)
	}
	if strings.TrimSpace(proj.WorkspacePath) == "" {
		return "", fmt.Errorf("workspace %s has empty path", workspaceID)
	}
	return proj.WorkspacePath, nil
}

// CreateRoomRequest is the body for POST /api/roundtable/rooms.
type CreateRoomRequest struct {
	Title string `json:"title"`
}

// CreateRoom allocates a room in drafting_brief, creates 6 kind=app seats
// (agent_type=grok-build), seeds each seat AGENTS.md, and starts a real
// Grok Build ChatSessionRecord on the referee seat (session index; ACP process
// starts on first R1 chat turn).
//
// kind=app keeps seats out of the sidebar 任务区 (which only lists workforce∪tmp).
func (svc *Service) CreateRoom(req CreateRoomRequest) (*Room, error) {
	if svc == nil || svc.store == nil {
		return nil, fmt.Errorf("roundtable: service not configured")
	}
	now := time.Now().UTC()
	roomID := meta.NewID()
	title := strings.TrimSpace(req.Title)
	if title == "" {
		title = "Agents 圆桌"
	}
	room := &Room{
		ID:        roomID,
		Title:     title,
		State:     StateDraftingBrief,
		CreatedAt: now,
		UpdatedAt: now,
		Seats:     make([]Seat, 0, len(DefaultRoster)),
	}

	// Create seats; on failure clean up disposable dirs already minted.
	var createdCwds []string
	cleanup := func() {
		for _, cwd := range createdCwds {
			_ = os.RemoveAll(cwd)
		}
	}

	for _, role := range DefaultRoster {
		idSuffix := fmt.Sprintf("rt-%s-%s", roomID, RoleSlug(role))
		displayName := fmt.Sprintf("圆桌·%s", RoleLabel(role))
		if title != "Agents 圆桌" {
			displayName = fmt.Sprintf("%s·%s", title, RoleLabel(role))
		}
		wsID, cwd, err := svc.createSeatWS(idSuffix, displayName)
		if err != nil {
			cleanup()
			return nil, fmt.Errorf("create app seat %s: %w", role, err)
		}
		createdCwds = append(createdCwds, cwd)
		if err := svc.writeSeed(cwd, role); err != nil {
			cleanup()
			return nil, fmt.Errorf("seed seat %s: %w", role, err)
		}
		seat := Seat{
			ID:          meta.NewID(),
			RoomID:      roomID,
			Role:        role,
			AgentType:   agent.AgentTypeGrokBuild,
			WorkspaceID: wsID,
			Status:      SeatReady,
			CreatedAt:   now,
		}
		// Sidecar lets seat agents resolve room_id from any nested cwd (CLI 跨 cwd).
		if err := WriteSeatSidecar(cwd, roomID, seat.ID, role); err != nil {
			cleanup()
			return nil, fmt.Errorf("seat sidecar %s: %w", role, err)
		}
		// Referee: register a real Grok Build session on create (design §5.2 R1).
		// Panelists get sessions on R2 first prompt (design §4: session 可预创建或 R2 再起).
		if role == RoleReferee {
			if err := svc.indexSeatSession(&seat, title, cwd); err != nil {
				cleanup()
				return nil, fmt.Errorf("start referee session: %w", err)
			}
		}
		room.Seats = append(room.Seats, seat)
	}

	if err := svc.store.InsertRoom(room); err != nil {
		cleanup()
		return nil, fmt.Errorf("persist room: %w", err)
	}
	if err := svc.projectRoomRuntime(room); err != nil {
		return nil, err
	}
	return room, nil
}

// indexSeatSession creates a ChatSessionRecord for a seat so it has a real
// Grok Build session id ready for prompt/resume.
func (svc *Service) indexSeatSession(seat *Seat, roomTitle, cwd string) error {
	if seat == nil {
		return fmt.Errorf("nil seat")
	}
	if svc.chatStore == nil {
		// Tests without a chat store still mint a session id for turn process_ref.
		seat.SessionID = meta.NewID()
		return nil
	}
	sessionID := meta.NewID()
	name := fmt.Sprintf("%s·%s", roomTitle, RoleLabel(seat.Role))
	rec := meta.ChatSessionRecord{
		ID:             sessionID,
		WorkspaceID:    seat.WorkspaceID,
		Name:           name,
		AgentType:      agent.AgentTypeGrokBuild,
		Cwd:            cwd,
		PermissionMode: "approve-all",
		// Session lives on a kind=app workspace — sidebar 任务区 excludes app seats.
		Role: "",
	}
	if err := svc.chatStore.Add(rec); err != nil {
		return err
	}
	seat.SessionID = sessionID
	return nil
}

// ListRooms returns room summaries (no seats/turns), newest first.
func (svc *Service) ListRooms(limit int) ([]Room, error) {
	if svc == nil || svc.store == nil {
		return nil, fmt.Errorf("roundtable: service not configured")
	}
	rooms, err := svc.store.ListRooms(limit)
	if err != nil {
		return nil, err
	}
	for i := range rooms {
		if err := svc.projectRoomRuntime(&rooms[i]); err != nil {
			return nil, err
		}
	}
	return rooms, nil
}

// GetRoom returns room + seats + turns (main timeline).
func (svc *Service) GetRoom(id string) (*Room, error) {
	room, err := svc.store.GetRoom(id)
	if err != nil {
		return nil, err
	}
	seats, err := svc.store.ListSeats(id)
	if err != nil {
		return nil, err
	}
	room.Seats = seats
	turns, err := svc.store.ListTurns(id)
	if err != nil {
		return nil, err
	}
	room.Turns = turns
	if err := svc.projectRoomRuntime(room); err != nil {
		return nil, err
	}
	return room, nil
}

// ListSeats returns seats for a room (error if room missing).
func (svc *Service) ListSeats(roomID string) ([]Seat, error) {
	if _, err := svc.store.GetRoom(roomID); err != nil {
		return nil, err
	}
	return svc.store.ListSeats(roomID)
}

// ListTurns returns the main timeline turns for a room.
func (svc *Service) ListTurns(roomID string) ([]Turn, error) {
	if _, err := svc.store.GetRoom(roomID); err != nil {
		return nil, err
	}
	return svc.store.ListTurns(roomID)
}

// TransitionRoom applies a legal state transition and persists it.
func (svc *Service) TransitionRoom(roomID string, to RoomState) (*Room, error) {
	room, err := svc.store.GetRoom(roomID)
	if err != nil {
		return nil, err
	}
	if err := Transition(room, to); err != nil {
		return nil, err
	}
	if err := svc.store.UpdateRoomState(room); err != nil {
		return nil, err
	}
	return svc.GetRoom(roomID)
}

// ChatRequest is the body for POST /api/roundtable/rooms/{id}/chat.
type ChatRequest struct {
	Text string `json:"text"`
}

// ChatResponse is the R1 chat turn pair plus session ids.
type ChatResponse struct {
	UserTurn     Turn   `json:"user_turn"`
	RefereeTurn  Turn   `json:"referee_turn"`
	SessionID    string `json:"session_id"`
	AcpSessionID string `json:"acp_session_id,omitempty"`
}

// r1RefereeSystemContext injects the complete referee role contract plus the
// current-stage instructions on the first R1 prompt (fresh ACP session).
func r1RefereeSystemContext() string {
	return rewriteRoundtableCLIInSeed(RoleSeedAGENTS(RoleReferee)) + `

---

## 当前阶段：R1 命题（用户 ↔ 裁判多轮）

你的目标：充分澄清议题，帮助用户形成可确认的 Brief，字段包括：
- title（议题标题）
- question（核心问题）
- constraints（约束）
- success_criteria（成功标准）
- product_kind（可选：software | hardware | hybrid）

规则：
- 只输出澄清对话/简短结构化进展正文，禁止寒暄。
- 不要替五职能席位做完整观点长文。
- 当信息足够时，给出一份建议 Brief 草案供用户确认。`
}

// ChatWithReferee runs one user↔referee R1 turn:
// writes user turn (kind=chat), prompts the real Grok Build session, writes
// referee turn with content_text for the main timeline.
func (svc *Service) ChatWithReferee(roomID string, req ChatRequest) (*ChatResponse, error) {
	if svc == nil || svc.store == nil {
		return nil, fmt.Errorf("roundtable: service not configured")
	}
	text := strings.TrimSpace(req.Text)
	if text == "" {
		return nil, fmt.Errorf("roundtable: chat text required")
	}

	room, err := svc.store.GetRoom(roomID)
	if err != nil {
		return nil, err
	}
	if room.State != StateDraftingBrief {
		return nil, fmt.Errorf("roundtable: chat only allowed in drafting_brief (state=%s)", room.State)
	}

	seats, err := svc.store.ListSeats(roomID)
	if err != nil {
		return nil, err
	}
	var referee *Seat
	for i := range seats {
		if seats[i].Role == RoleReferee {
			referee = &seats[i]
			break
		}
	}
	if referee == nil {
		return nil, fmt.Errorf("roundtable: referee seat missing")
	}

	// Ensure Grok Build session index exists (CreateRoom normally seeds it).
	if strings.TrimSpace(referee.SessionID) == "" {
		cwd, err := svc.resolveCwd(referee.WorkspaceID)
		if err != nil {
			return nil, fmt.Errorf("resolve referee cwd: %w", err)
		}
		if err := svc.indexSeatSession(referee, room.Title, cwd); err != nil {
			return nil, fmt.Errorf("start referee session: %w", err)
		}
		if err := svc.store.UpdateSeatSession(referee); err != nil {
			return nil, err
		}
	}

	cwd, err := svc.resolveCwd(referee.WorkspaceID)
	if err != nil {
		return nil, fmt.Errorf("resolve referee cwd: %w", err)
	}

	now := time.Now().UTC()
	userTurn := Turn{
		ID:          meta.NewID(),
		RoomID:      roomID,
		Round:       1,
		SeatID:      TurnSeatUser,
		Kind:        TurnKindChat,
		ContentText: text,
		CreatedAt:   now,
	}
	if err := svc.store.InsertTurn(&userTurn); err != nil {
		return nil, err
	}

	// Mark speaking while the model turn runs.
	referee.Status = SeatSpeaking
	_ = svc.store.UpdateSeatSession(referee)

	sysCtx := ""
	if referee.AcpSessionID == "" {
		sysCtx = r1RefereeSystemContext()
	}
	result, err := svc.prompter.Prompt(SeatPromptRequest{
		SessionID:      referee.SessionID,
		WorkspacePath:  cwd,
		AgentType:      referee.AgentType,
		AcpSessionID:   referee.AcpSessionID,
		Text:           text,
		SystemContext:  sysCtx,
		PermissionMode: "approve-all",
		Role:           RoleReferee,
	})
	if err != nil {
		referee.Status = SeatReady
		_ = svc.store.UpdateSeatSession(referee)
		return nil, fmt.Errorf("referee prompt: %w", err)
	}

	if result.AcpSessionID != "" {
		referee.AcpSessionID = result.AcpSessionID
		if svc.chatStore != nil && referee.SessionID != "" {
			_ = svc.chatStore.UpdateACP(referee.SessionID, result.AcpSessionID)
		}
	}
	referee.Status = SeatReady
	if err := svc.store.UpdateSeatSession(referee); err != nil {
		return nil, err
	}

	replyText := strings.TrimSpace(result.Text)
	if replyText == "" {
		replyText = "（裁判本轮无正文输出）"
	}
	refTurn := Turn{
		ID:          meta.NewID(),
		RoomID:      roomID,
		Round:       1,
		SeatID:      referee.ID,
		Kind:        TurnKindChat,
		ContentText: replyText,
		// process_ref points at the underlying Grok Build chat session for fold-out process.
		ProcessRef: referee.SessionID,
		CreatedAt:  time.Now().UTC(),
	}
	if err := svc.store.InsertTurn(&refTurn); err != nil {
		return nil, err
	}

	return &ChatResponse{
		UserTurn:     userTurn,
		RefereeTurn:  refTurn,
		SessionID:    referee.SessionID,
		AcpSessionID: referee.AcpSessionID,
	}, nil
}

// ConfirmBriefRequest is the legacy management body for POST .../{id}/brief
// and `set-brief`. New clients create a version and confirm it separately.
type ConfirmBriefRequest struct {
	Title           string      `json:"title"`
	Question        string      `json:"question"`
	Constraints     string      `json:"constraints"`
	SuccessCriteria string      `json:"success_criteria"`
	ProductKind     ProductKind `json:"product_kind,omitempty"`
}

// SaveBriefDraftRequest creates a user-authored draft version.
type SaveBriefDraftRequest struct {
	ConfirmBriefRequest
	ExpectedVersion int `json:"expected_version"`
}

// ProposeBriefRequest is the only Brief write exposed to referee agents.
type ProposeBriefRequest struct {
	ConfirmBriefRequest
	ExpectedVersion int    `json:"expected_version"`
	SourceTurnID    string `json:"source_turn_id,omitempty"`
}

// ConfirmBriefVersionRequest confirms an existing current version. It carries
// no content, so confirmation cannot silently overwrite a newer proposal.
type ConfirmBriefVersionRequest struct {
	Version         int `json:"version"`
	ExpectedVersion int `json:"expected_version"`
}

// isPlaceholderBriefField reports empty or pure placeholder values that must
// never enter R2 (e.g. UI silent fill of "—" / "圆桌议题").
func isPlaceholderBriefField(s string) bool {
	v := strings.TrimSpace(s)
	if v == "" {
		return true
	}
	switch v {
	case "—", "-", "–", "−", "N/A", "n/a", "NA", "na", "TODO", "todo", "TBD", "tbd":
		return true
	}
	return false
}

// ValidateBrief checks minimum Brief fields (design §4 R1).
// Rejects empty fields and pure placeholders so R2 never sees unusable Briefs.
func ValidateBrief(b *Brief) error {
	if b == nil {
		return fmt.Errorf("brief is required")
	}
	if isPlaceholderBriefField(b.Title) {
		return fmt.Errorf("brief.title is required (placeholder values like \"—\" rejected)")
	}
	if isPlaceholderBriefField(b.Question) {
		return fmt.Errorf("brief.question is required (placeholder values like \"—\" rejected)")
	}
	if isPlaceholderBriefField(b.Constraints) {
		return fmt.Errorf("brief.constraints is required (placeholder values like \"—\" rejected)")
	}
	if isPlaceholderBriefField(b.SuccessCriteria) {
		return fmt.Errorf("brief.success_criteria is required (placeholder values like \"—\" rejected)")
	}
	if b.ProductKind != "" {
		switch b.ProductKind {
		case ProductSoftware, ProductHardware, ProductHybrid:
		default:
			return fmt.Errorf("brief.product_kind must be software|hardware|hybrid")
		}
	}
	return nil
}

func briefFromRequest(req ConfirmBriefRequest) *Brief {
	return &Brief{
		Title:           strings.TrimSpace(req.Title),
		Question:        strings.TrimSpace(req.Question),
		Constraints:     strings.TrimSpace(req.Constraints),
		SuccessCriteria: strings.TrimSpace(req.SuccessCriteria),
		ProductKind:     ProductKind(strings.TrimSpace(string(req.ProductKind))),
	}
}

// SaveBriefDraft appends a user-authored draft using optimistic versioning.
func (svc *Service) SaveBriefDraft(roomID string, req SaveBriefDraftRequest) (*Room, error) {
	if svc == nil || svc.store == nil {
		return nil, fmt.Errorf("roundtable: service not configured")
	}
	brief := briefFromRequest(req.ConfirmBriefRequest)
	if err := ValidateBrief(brief); err != nil {
		return nil, err
	}
	if _, err := svc.store.CreateBriefVersion(
		roomID,
		req.ExpectedVersion,
		BriefStatusDraft,
		*brief,
		BriefProposerUser,
		"",
	); err != nil {
		return nil, err
	}
	return svc.GetRoom(roomID)
}

// ProposeBrief appends a referee proposal. The request has no status or
// confirmation field, so an agent cannot turn its proposal into confirmation.
func (svc *Service) ProposeBrief(roomID string, req ProposeBriefRequest) (*Room, error) {
	if svc == nil || svc.store == nil {
		return nil, fmt.Errorf("roundtable: service not configured")
	}
	brief := briefFromRequest(req.ConfirmBriefRequest)
	if err := ValidateBrief(brief); err != nil {
		return nil, err
	}
	version, err := svc.store.CreateBriefVersion(
		roomID,
		req.ExpectedVersion,
		BriefStatusProposed,
		*brief,
		BriefProposerReferee,
		req.SourceTurnID,
	)
	if err != nil {
		return nil, err
	}
	sys := Turn{
		ID:          meta.NewID(),
		RoomID:      roomID,
		Round:       1,
		Kind:        TurnKindSystem,
		ContentText: fmt.Sprintf("Brief 草案已更新至 v%d，等待用户确认。", version.Version),
		ProcessRef:  strings.TrimSpace(req.SourceTurnID),
		CreatedAt:   time.Now().UTC(),
	}
	if err := svc.store.InsertTurn(&sys); err != nil {
		return nil, err
	}
	return svc.GetRoom(roomID)
}

// ConfirmBriefVersion is the user confirmation path. It confirms exactly the
// current version and rejects stale expected_version values.
func (svc *Service) ConfirmBriefVersion(roomID string, req ConfirmBriefVersionRequest) (*Room, error) {
	if svc == nil || svc.store == nil {
		return nil, fmt.Errorf("roundtable: service not configured")
	}
	if req.Version <= 0 {
		return nil, fmt.Errorf("brief.version is required")
	}
	version, err := svc.store.ConfirmBriefVersion(roomID, req.Version, req.ExpectedVersion)
	if err != nil {
		return nil, err
	}
	sys := Turn{
		ID:          meta.NewID(),
		RoomID:      roomID,
		Round:       1,
		SeatID:      TurnSeatUser,
		Kind:        TurnKindSystem,
		ContentText: fmt.Sprintf("你已确认 Brief v%d，进入 R2。", version.Version),
		CreatedAt:   time.Now().UTC(),
	}
	if err := svc.store.InsertTurn(&sys); err != nil {
		return nil, err
	}
	return svc.GetRoom(roomID)
}

// ConfirmBrief preserves the old one-shot management contract. It creates a
// user proposal and confirms that exact version; agents use ProposeBrief.
func (svc *Service) ConfirmBrief(roomID string, req ConfirmBriefRequest) (*Room, error) {
	if svc == nil || svc.store == nil {
		return nil, fmt.Errorf("roundtable: service not configured")
	}
	brief := briefFromRequest(req)
	if err := ValidateBrief(brief); err != nil {
		return nil, err
	}
	room, err := svc.store.GetRoom(roomID)
	if err != nil {
		return nil, err
	}
	version, err := svc.store.CreateBriefVersion(
		roomID,
		room.CurrentBriefVersion,
		BriefStatusProposed,
		*brief,
		BriefProposerUser,
		"",
	)
	if err != nil {
		return nil, err
	}
	return svc.ConfirmBriefVersion(roomID, ConfirmBriefVersionRequest{
		Version:         version.Version,
		ExpectedVersion: version.Version,
	})
}

// ---------------------------------------------------------------------------
// R2: five isolated panelist speeches + referee Summary₂ (design §4 R2)
// ---------------------------------------------------------------------------

// r2PanelistSystemContext is injected on each panelist's first (only) R2 turn.
// Must NOT include any other seat's output — isolation is the hard R2 rule.
func r2PanelistSystemContext(role Role) string {
	return RoleSeedAGENTS(role) + "\n\n---\n\n" + fmt.Sprintf(`## 当前阶段：R2 首轮各自发言

你的职能席位：%s（%s）。
规则：
- 只输出本职能观点正文，禁止寒暄。
- 紧扣 Brief；你看不到其他席位的发言，也不要假设他人观点。
- 恰好 1 次本轮发言；Tool/thinking 进 process，不进 content_text。`,
		RoleLabel(role), role)
}

// FormatBriefForPrompt renders Brief fields for injection (R2 isolation: only this + role).
func FormatBriefForPrompt(b *Brief) string {
	if b == nil {
		return "【Brief】\n（空）"
	}
	var sb strings.Builder
	sb.WriteString("【Brief】\n")
	sb.WriteString("title: " + b.Title + "\n")
	sb.WriteString("question: " + b.Question + "\n")
	sb.WriteString("constraints: " + b.Constraints + "\n")
	sb.WriteString("success_criteria: " + b.SuccessCriteria + "\n")
	if b.ProductKind != "" {
		sb.WriteString("product_kind: " + string(b.ProductKind) + "\n")
	}
	sb.WriteString("\n请基于以上 Brief，从你的职能视角输出本轮首轮发言正文。")
	return sb.String()
}

// BuildR2PanelistPrompt returns the user prompt text for one R2 panelist turn.
// Isolation guarantee: only Brief content — never other seats' speeches.
func BuildR2PanelistPrompt(brief *Brief) string {
	return FormatBriefForPrompt(brief)
}

// r2SpeechItem is one panelist's R2 result for summary packaging.
type r2SpeechItem struct {
	Role    Role
	SeatID  string
	Text    string
	Failed  bool
	FailMsg string
}

// BuildR2SummaryPrompt builds the referee's Summary₂ prompt from available speeches.
func BuildR2SummaryPrompt(brief *Brief, items []r2SpeechItem) string {
	var sb strings.Builder
	sb.WriteString("当前阶段：R2 末总结（Summary₂）。\n\n")
	sb.WriteString(FormatBriefForPrompt(brief))
	sb.WriteString("\n\n【各席 R2 首轮发言】\n")
	for _, it := range items {
		label := RoleLabel(it.Role)
		if it.Failed {
			sb.WriteString(fmt.Sprintf("\n### %s（缺席/失败）\n%s\n", label, it.FailMsg))
			continue
		}
		sb.WriteString(fmt.Sprintf("\n### %s\n%s\n", label, it.Text))
	}
	sb.WriteString(`
请输出 Summary₂ 正文：
- 综合各席要点，并标注观点来源席位
- 冲突时显式写出分歧（尤其产品诉求 vs 技术约束）
- 注明缺席/失败席位
- 禁止寒暄；不代替 panelist 重写长文`)
	return sb.String()
}

// appendMissingRoleRecord makes absence part of the persisted artifact instead
// of relying on the referee model to follow the prompt perfectly.
func appendMissingRoleRecord(summary string, items []r2SpeechItem) string {
	var missing []string
	for _, item := range items {
		if item.Failed {
			missing = append(missing, fmt.Sprintf("- %s：%s", RoleLabel(item.Role), strings.TrimSpace(item.FailMsg)))
		}
	}
	if len(missing) == 0 {
		return summary
	}
	return strings.TrimSpace(summary) + "\n\n## 缺席角色（系统记录）\n" + strings.Join(missing, "\n")
}

// RunR2Response is the result of POST .../r2.
type RunR2Response struct {
	Room        *Room    `json:"room"`
	SpeechTurns []Turn   `json:"speech_turns"`
	SummaryTurn Turn     `json:"summary_turn"`
	FailedRoles []string `json:"failed_roles,omitempty"`
}

// panelistPromptResult is an in-memory result from one parallel R2 seat turn.
type panelistPromptResult struct {
	seat       Seat
	text       string
	acpID      string
	err        error
	injectText string // prompt Text actually sent (for isolation audits)
	injectSys  string
}

// RunR2 executes design §4 R2:
//  1. Parallel prompt market/product/eng/ops/finance with Brief+role only (互不可见)
//  2. Extract content_text → turns kind=speech; failed seats marked failed (不阻断)
//  3. Referee Summary₂ (kind=summary); room.summary_r2 set; state → waiting_r3
func (svc *Service) RunR2(roomID string) (*RunR2Response, error) {
	return svc.runR2(roomID, nil)
}

// runR2 executes either the original direct service path (run=nil) or a run
// that was already atomically claimed by StartR2.
func (svc *Service) runR2(roomID string, run *RoundRun) (*RunR2Response, error) {
	if svc == nil || svc.store == nil {
		return nil, fmt.Errorf("roundtable: service not configured")
	}
	room, err := svc.store.GetRoom(roomID)
	if err != nil {
		return nil, err
	}
	var briefVersion *BriefVersion
	if run == nil {
		if room.State != StateWaitingR2 {
			return nil, fmt.Errorf("roundtable: r2 only allowed in waiting_r2 (state=%s)", room.State)
		}
		briefVersion, err = svc.store.CaptureConfirmedBriefForR2(roomID)
		if err != nil {
			return nil, err
		}
	} else {
		if run.RoomID != roomID || run.Round != 2 {
			return nil, fmt.Errorf("roundtable: invalid claimed r2 run")
		}
		if room.State != StateSummarizingR2 {
			return nil, fmt.Errorf("roundtable: claimed r2 requires summarizing_r2 (state=%s)", room.State)
		}
		if room.R2BriefVersion <= 0 {
			return nil, fmt.Errorf("roundtable: r2 brief snapshot required")
		}
		briefVersion, err = svc.store.GetBriefVersion(roomID, room.R2BriefVersion)
		if err != nil {
			return nil, err
		}
	}
	briefSnapshot := briefVersion.Content
	if err := ValidateBrief(&briefSnapshot); err != nil {
		return nil, fmt.Errorf("roundtable: %w", err)
	}
	room.R2BriefVersion = briefVersion.Version
	room.R2Brief = briefVersion

	seats, err := svc.store.ListSeats(roomID)
	if err != nil {
		return nil, err
	}
	panelists := make([]Seat, 0, len(PanelistRoles))
	var referee *Seat
	for i := range seats {
		s := seats[i]
		if s.Role == RoleReferee {
			// copy for later mutation
			ref := s
			referee = &ref
			continue
		}
		if IsPanelist(s.Role) {
			panelists = append(panelists, s)
		}
	}
	if referee == nil {
		return nil, fmt.Errorf("roundtable: referee seat missing")
	}
	if len(panelists) != 5 {
		return nil, fmt.Errorf("roundtable: expected 5 panelists, got %d", len(panelists))
	}

	// Ensure chat sessions + cwd for each panelist serially (sqlite-safe), then
	// run model turns in parallel. Prompt isolation is structural: each inject
	// is Brief+role only — never peer speech bodies.
	prepared := make([]Seat, len(panelists))
	cwds := make([]string, len(panelists))
	for i, seat := range panelists {
		cwd, err := svc.resolveCwd(seat.WorkspaceID)
		if err != nil {
			return nil, fmt.Errorf("resolve cwd %s: %w", seat.Role, err)
		}
		if strings.TrimSpace(seat.SessionID) == "" {
			if err := svc.indexSeatSession(&seat, room.Title, cwd); err != nil {
				return nil, fmt.Errorf("index session %s: %w", seat.Role, err)
			}
		}
		if run != nil {
			started, err := svc.store.StartRunSeat(run.ID, seat.Role)
			if err != nil {
				return nil, err
			}
			if !started {
				return nil, fmt.Errorf("roundtable: r2 seat %s already started", seat.Role)
			}
		}
		seat.Status = SeatSpeaking
		if err := svc.store.UpdateSeatSession(&seat); err != nil {
			return nil, err
		}
		prepared[i] = seat
		cwds[i] = cwd
	}

	// --- Parallel isolated panelist prompts ---
	results := make([]panelistPromptResult, len(prepared))
	var wg sync.WaitGroup
	for i := range prepared {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i] = svc.runR2PanelistPrompt(&briefSnapshot, prepared[i], cwds[i])
		}(i)
	}
	wg.Wait()

	// Persist speech turns + seat status serially (sqlite-friendly).
	speechTurns := make([]Turn, 0, len(results))
	items := make([]r2SpeechItem, 0, len(results))
	var failedRoles []string
	for _, r := range results {
		seat := r.seat
		if r.err != nil {
			seat.Status = SeatFailed
			if r.acpID != "" {
				seat.AcpSessionID = r.acpID
			}
			_ = svc.store.UpdateSeatSession(&seat)

			failMsg := fmt.Sprintf("[failed] %s 席 R2 发言失败：%v", RoleLabel(seat.Role), r.err)
			turn := Turn{
				ID:          meta.NewID(),
				RoomID:      roomID,
				Round:       2,
				SeatID:      seat.ID,
				Kind:        TurnKindSpeech,
				ContentText: failMsg,
				ProcessRef:  seat.SessionID,
				CreatedAt:   time.Now().UTC(),
			}
			if err := svc.store.InsertTurn(&turn); err != nil {
				return nil, err
			}
			if run != nil {
				if err := svc.store.FinishRunSeat(run.ID, seat.Role, true, r.err.Error()); err != nil {
					return nil, err
				}
			}
			speechTurns = append(speechTurns, turn)
			items = append(items, r2SpeechItem{
				Role: seat.Role, SeatID: seat.ID, Failed: true, FailMsg: failMsg,
			})
			failedRoles = append(failedRoles, string(seat.Role))
			log.Printf("[roundtable] R2 seat failed room=%s role=%s err=%v", roomID, seat.Role, r.err)
			continue
		}

		seat.Status = SeatDone
		if r.acpID != "" {
			seat.AcpSessionID = r.acpID
			if svc.chatStore != nil && seat.SessionID != "" {
				_ = svc.chatStore.UpdateACP(seat.SessionID, r.acpID)
			}
		}
		if err := svc.store.UpdateSeatSession(&seat); err != nil {
			return nil, err
		}

		text := strings.TrimSpace(r.text)
		if text == "" {
			text = fmt.Sprintf("（%s 本轮无正文输出）", RoleLabel(seat.Role))
		}
		turn := Turn{
			ID:          meta.NewID(),
			RoomID:      roomID,
			Round:       2,
			SeatID:      seat.ID,
			Kind:        TurnKindSpeech,
			ContentText: text,
			ProcessRef:  seat.SessionID,
			CreatedAt:   time.Now().UTC(),
		}
		if err := svc.store.InsertTurn(&turn); err != nil {
			return nil, err
		}

		// === 立即写回结论：对话结束就拉最新回复持久化 ===
		if run != nil {
			if err := svc.store.FinishRunSeat(run.ID, seat.Role, false, ""); err != nil {
				return nil, err
			}
		}
		speechTurns = append(speechTurns, turn)
		items = append(items, r2SpeechItem{
			Role: seat.Role, SeatID: seat.ID, Text: text,
		})
	}

	if run != nil && len(failedRoles) > 0 {
		if err := svc.store.PauseRoundRunForSeats(run.ID); err != nil {
			return nil, err
		}
		full, err := svc.GetRoom(roomID)
		if err != nil {
			return nil, err
		}
		return &RunR2Response{
			Room:        full,
			SpeechTurns: speechTurns,
			FailedRoles: failedRoles,
		}, nil
	}

	// --- summarizing_r2 ---
	if run == nil {
		if err := Transition(room, StateSummarizingR2); err != nil {
			return nil, err
		}
		if err := svc.store.UpdateRoomState(room); err != nil {
			return nil, err
		}
	} else if err := svc.store.UpdateRunStatus(run.ID, RunSummarizing, ""); err != nil {
		return nil, err
	}

	// --- Referee Summary₂ ---
	summaryTurn, summaryText, err := svc.runR2RefereeSummary(room, &briefSnapshot, referee, items)
	if err != nil {
		// Room-level failure only if summary cannot complete after speeches.
		if run == nil {
			_ = Transition(room, StateFailed)
			_ = svc.store.UpdateRoomState(room)
		}
		return nil, fmt.Errorf("r2 summary: %w", err)
	}

	room.SummaryR2 = summaryText
	if run == nil {
		if err := Transition(room, StateWaitingR3); err != nil {
			return nil, err
		}
		if err := svc.store.UpdateRoomSummaryR2AndState(room); err != nil {
			return nil, err
		}
	} else {
		status := RunCompleted
		if len(failedRoles) > 0 {
			status = RunPartialFailed
		}
		if err := svc.store.FinalizeRoundRun(
			run.ID,
			status,
			StateWaitingR3,
			summaryText,
			"",
			RunErrorNone,
		); err != nil {
			return nil, err
		}
	}

	full, err := svc.GetRoom(roomID)
	if err != nil {
		return nil, err
	}
	return &RunR2Response{
		Room:        full,
		SpeechTurns: speechTurns,
		SummaryTurn: summaryTurn,
		FailedRoles: failedRoles,
	}, nil
}

// runR2PanelistPrompt prompts one prepared panelist with Brief+role only.
// Safe to call concurrently: does not write to the store.
func (svc *Service) runR2PanelistPrompt(brief *Brief, seat Seat, cwd string) panelistPromptResult {
	out := panelistPromptResult{seat: seat}

	sysCtx := r2PanelistSystemContext(seat.Role)
	promptText := BuildR2PanelistPrompt(brief)
	out.injectText = promptText
	out.injectSys = sysCtx

	// Isolation audit log: prove inject is brief+role only (no peer speech bodies).
	log.Printf("[roundtable] R2 inject isolation=brief+role_only room_seat=%s role=%s session=%s text_len=%d sys_len=%d peer_speech=forbidden",
		seat.ID, seat.Role, seat.SessionID, len(promptText), len(sysCtx))

	result, err := svc.prompter.Prompt(SeatPromptRequest{
		SessionID:      seat.SessionID,
		WorkspacePath:  cwd,
		AgentType:      seat.AgentType,
		AcpSessionID:   seat.AcpSessionID, // empty on first R2 turn
		Text:           promptText,
		SystemContext:  sysCtx,
		PermissionMode: "approve-all",
		Role:           seat.Role,
	})
	if err != nil {
		out.err = err
		return out
	}
	out.text = result.Text
	out.acpID = result.AcpSessionID
	return out
}

// runR2RefereeSummary prompts the referee with all available Speech₂ texts.
func (svc *Service) runR2RefereeSummary(room *Room, brief *Brief, referee *Seat, items []r2SpeechItem) (Turn, string, error) {
	if strings.TrimSpace(referee.SessionID) == "" {
		cwd, err := svc.resolveCwd(referee.WorkspaceID)
		if err != nil {
			return Turn{}, "", fmt.Errorf("resolve referee cwd: %w", err)
		}
		if err := svc.indexSeatSession(referee, room.Title, cwd); err != nil {
			return Turn{}, "", err
		}
		if err := svc.store.UpdateSeatSession(referee); err != nil {
			return Turn{}, "", err
		}
	}
	cwd, err := svc.resolveCwd(referee.WorkspaceID)
	if err != nil {
		return Turn{}, "", fmt.Errorf("resolve referee cwd: %w", err)
	}

	referee.Status = SeatSpeaking
	_ = svc.store.UpdateSeatSession(referee)

	promptText := BuildR2SummaryPrompt(brief, items)
	// Resume R1 referee session when possible; system context only if fresh.
	sysCtx := ""
	if referee.AcpSessionID == "" {
		sysCtx = "当前阶段：R2 末裁判总结。只输出 Summary₂ 正文。"
	}
	result, err := svc.prompter.Prompt(SeatPromptRequest{
		SessionID:      referee.SessionID,
		WorkspacePath:  cwd,
		AgentType:      referee.AgentType,
		AcpSessionID:   referee.AcpSessionID,
		Text:           promptText,
		SystemContext:  sysCtx,
		PermissionMode: "approve-all",
		Role:           RoleReferee,
	})
	if err != nil {
		referee.Status = SeatReady
		_ = svc.store.UpdateSeatSession(referee)
		return Turn{}, "", err
	}
	if result.AcpSessionID != "" {
		referee.AcpSessionID = result.AcpSessionID
		if svc.chatStore != nil && referee.SessionID != "" {
			_ = svc.chatStore.UpdateACP(referee.SessionID, result.AcpSessionID)
		}
	}
	referee.Status = SeatReady
	if err := svc.store.UpdateSeatSession(referee); err != nil {
		return Turn{}, "", err
	}

	summaryText := strings.TrimSpace(result.Text)
	if summaryText == "" {
		summaryText = "（裁判 Summary₂ 无正文输出）"
	}
	summaryText = appendMissingRoleRecord(summaryText, items)

	turn := Turn{
		ID:          meta.NewID(),
		RoomID:      room.ID,
		Round:       2,
		SeatID:      referee.ID,
		Kind:        TurnKindSummary,
		ContentText: summaryText,
		ProcessRef:  referee.SessionID,
		CreatedAt:   time.Now().UTC(),
	}
	if err := svc.store.InsertTurn(&turn); err != nil {
		return Turn{}, "", err
	}

	room.SummaryR2 = summaryText
	return turn, summaryText, nil
}

// ---------------------------------------------------------------------------
// R3: resume same acp_session_id + public context + referee Summary₃ → done
// (design §4 R3 / §5.2)
// ---------------------------------------------------------------------------

// FormatBriefHeader renders Brief fields without the R2-only closing instruction.
func FormatBriefHeader(b *Brief) string {
	if b == nil {
		return "【Brief】\n（空）"
	}
	var sb strings.Builder
	sb.WriteString("【Brief】\n")
	sb.WriteString("title: " + b.Title + "\n")
	sb.WriteString("question: " + b.Question + "\n")
	sb.WriteString("constraints: " + b.Constraints + "\n")
	sb.WriteString("success_criteria: " + b.SuccessCriteria + "\n")
	if b.ProductKind != "" {
		sb.WriteString("product_kind: " + string(b.ProductKind) + "\n")
	}
	return sb.String()
}

// FormatSpeechPackage renders speech bodies for public injection (content_text only;
// never tool traces / process).
func FormatSpeechPackage(title string, items []r2SpeechItem) string {
	var sb strings.Builder
	sb.WriteString(title)
	sb.WriteString("\n")
	for _, it := range items {
		label := RoleLabel(it.Role)
		if it.Failed {
			sb.WriteString(fmt.Sprintf("\n### %s（缺席/失败）\n%s\n", label, it.FailMsg))
			continue
		}
		sb.WriteString(fmt.Sprintf("\n### %s\n%s\n", label, it.Text))
	}
	return sb.String()
}

// BuildR3PanelistPrompt builds the R3 public context package (design §4 R3):
// Brief + 五席 Speech₂ 全文 + Summary₂（仅正文，无 tool trace）.
// Role instructions live in the user prompt because resume does not re-inject SystemContext.
func BuildR3PanelistPrompt(role Role, brief *Brief, r2Items []r2SpeechItem, summaryR2 string) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf(`当前阶段：R3 次轮各自发言。

你的职能席位：%s（%s）。
规则：
- 只输出本职能次轮观点正文，禁止寒暄。
- 你可看到 Brief、五席 R2 发言正文与裁判 Summary₂；请在此基础上回应、补充或反驳。
- 恰好 1 次本轮发言；禁止把 tool/thinking 写入正文。

`, RoleLabel(role), role))
	sb.WriteString(FormatBriefHeader(brief))
	sb.WriteString("\n")
	sb.WriteString(FormatSpeechPackage("【各席 R2 首轮发言全文】", r2Items))
	sb.WriteString("\n【Summary₂】\n")
	if strings.TrimSpace(summaryR2) == "" {
		sb.WriteString("（无）\n")
	} else {
		sb.WriteString(strings.TrimSpace(summaryR2))
		sb.WriteString("\n")
	}
	sb.WriteString("\n请基于以上公开上下文，从你的职能视角输出本轮（R3）次轮发言正文。")
	return sb.String()
}

// BuildR3SummaryPrompt builds the referee's Summary₃ (终稿) prompt from R3 speeches.
func BuildR3SummaryPrompt(brief *Brief, r2Items []r2SpeechItem, summaryR2 string, r3Items []r2SpeechItem) string {
	var sb strings.Builder
	sb.WriteString("当前阶段：R3 末终稿（Summary₃）。\n\n")
	sb.WriteString(FormatBriefHeader(brief))
	sb.WriteString("\n")
	sb.WriteString(FormatSpeechPackage("【各席 R2 首轮发言】", r2Items))
	sb.WriteString("\n【Summary₂】\n")
	if strings.TrimSpace(summaryR2) == "" {
		sb.WriteString("（无）\n")
	} else {
		sb.WriteString(strings.TrimSpace(summaryR2))
		sb.WriteString("\n")
	}
	sb.WriteString("\n")
	sb.WriteString(FormatSpeechPackage("【各席 R3 次轮发言】", r3Items))
	sb.WriteString(`
请输出 Summary₃ 终稿正文：
- 综合两轮要点，标注观点来源席位
- 冲突时显式写出分歧（尤其产品诉求 vs 技术约束）
- 注明缺席/失败席位
- 给出可执行的收敛结论（终稿）
- 禁止寒暄；不代替 panelist 重写长文`)
	return sb.String()
}

// RunR3Response is the result of POST .../r3.
type RunR3Response struct {
	Room        *Room    `json:"room"`
	SpeechTurns []Turn   `json:"speech_turns"`
	SummaryTurn Turn     `json:"summary_turn"`
	FailedRoles []string `json:"failed_roles,omitempty"`
}

// collectR2SpeechItems loads round-2 speech turns and maps them to seats for the
// public context package (content_text only).
func (svc *Service) collectR2SpeechItems(roomID string, seats []Seat) ([]r2SpeechItem, error) {
	turns, err := svc.store.ListTurns(roomID)
	if err != nil {
		return nil, err
	}
	seatByID := map[string]Seat{}
	for _, s := range seats {
		seatByID[s.ID] = s
	}
	// Preserve PanelistRoles order for stable injection.
	byRole := map[Role]r2SpeechItem{}
	for _, tr := range turns {
		if tr.Round != 2 || tr.Kind != TurnKindSpeech {
			continue
		}
		seat, ok := seatByID[tr.SeatID]
		if !ok || !IsPanelist(seat.Role) {
			continue
		}
		item := r2SpeechItem{Role: seat.Role, SeatID: seat.ID, Text: tr.ContentText}
		if strings.HasPrefix(tr.ContentText, "[failed]") {
			item.Failed = true
			item.FailMsg = tr.ContentText
		}
		byRole[seat.Role] = item
	}
	out := make([]r2SpeechItem, 0, len(PanelistRoles))
	for _, role := range PanelistRoles {
		if it, ok := byRole[role]; ok {
			out = append(out, it)
			continue
		}
		// Missing speech turn: still list seat as absent so others see the gap.
		var seatID string
		for _, s := range seats {
			if s.Role == role {
				seatID = s.ID
				break
			}
		}
		out = append(out, r2SpeechItem{
			Role: role, SeatID: seatID, Failed: true,
			FailMsg: fmt.Sprintf("[failed] %s 席 R2 无发言记录", RoleLabel(role)),
		})
	}
	return out, nil
}

// RunR3 executes design §4 R3 / §5.2:
//  1. Each panelist session/resume 同一 acp_session_id（跨 R2→R3 持久化）
//  2. Inject Brief + 五席 Speech₂ 全文 + Summary₂（仅正文，无 tool trace）
//  3. 各 1 speech turn；裁判 Summary₃；state=done
func (svc *Service) RunR3(roomID string) (*RunR3Response, error) {
	return svc.runR3(roomID, nil)
}

// runR3 executes either the original direct service path (run=nil) or a run
// that was already atomically claimed by StartR3.
func (svc *Service) runR3(roomID string, run *RoundRun) (*RunR3Response, error) {
	if svc == nil || svc.store == nil {
		return nil, fmt.Errorf("roundtable: service not configured")
	}
	room, err := svc.store.GetRoom(roomID)
	if err != nil {
		return nil, err
	}
	wantState := StateWaitingR3
	if run != nil {
		if run.RoomID != roomID || run.Round != 3 {
			return nil, fmt.Errorf("roundtable: invalid claimed r3 run")
		}
		wantState = StateSummarizingR3
	}
	if room.State != wantState {
		return nil, fmt.Errorf("roundtable: r3 only allowed in %s (state=%s)", wantState, room.State)
	}
	if room.R2BriefVersion <= 0 {
		return nil, fmt.Errorf("roundtable: r2 brief snapshot required before r3")
	}
	r2BriefVersion, err := svc.store.GetBriefVersion(roomID, room.R2BriefVersion)
	if err != nil {
		return nil, fmt.Errorf("roundtable: load r2 brief snapshot: %w", err)
	}
	briefSnapshot := r2BriefVersion.Content
	if err := ValidateBrief(&briefSnapshot); err != nil {
		return nil, fmt.Errorf("roundtable: %w", err)
	}
	if strings.TrimSpace(room.SummaryR2) == "" {
		return nil, fmt.Errorf("roundtable: summary_r2 required before r3")
	}

	seats, err := svc.store.ListSeats(roomID)
	if err != nil {
		return nil, err
	}
	panelists := make([]Seat, 0, len(PanelistRoles))
	var referee *Seat
	for i := range seats {
		s := seats[i]
		if s.Role == RoleReferee {
			ref := s
			referee = &ref
			continue
		}
		if IsPanelist(s.Role) {
			panelists = append(panelists, s)
		}
	}
	if referee == nil {
		return nil, fmt.Errorf("roundtable: referee seat missing")
	}
	if len(panelists) != 5 {
		return nil, fmt.Errorf("roundtable: expected 5 panelists, got %d", len(panelists))
	}

	r2Items, err := svc.collectR2SpeechItems(roomID, seats)
	if err != nil {
		return nil, err
	}

	// Resolve cwd + mark speaking; acp_session_id must already be persisted from R2.
	prepared := make([]Seat, len(panelists))
	cwds := make([]string, len(panelists))
	for i, seat := range panelists {
		cwd, err := svc.resolveCwd(seat.WorkspaceID)
		if err != nil {
			return nil, fmt.Errorf("resolve cwd %s: %w", seat.Role, err)
		}
		if strings.TrimSpace(seat.SessionID) == "" {
			// R3 expects R2 to have indexed the session; create as last resort.
			if err := svc.indexSeatSession(&seat, room.Title, cwd); err != nil {
				return nil, fmt.Errorf("index session %s: %w", seat.Role, err)
			}
		}
		if run != nil {
			started, err := svc.store.StartRunSeat(run.ID, seat.Role)
			if err != nil {
				return nil, err
			}
			if !started {
				return nil, fmt.Errorf("roundtable: r3 seat %s already started", seat.Role)
			}
		}
		// Seats that never got an ACP session in R2 cannot resume — fail later in parallel path.
		seat.Status = SeatSpeaking
		if err := svc.store.UpdateSeatSession(&seat); err != nil {
			return nil, err
		}
		prepared[i] = seat
		cwds[i] = cwd
	}

	// --- Parallel R3 panelist prompts (resume + public context) ---
	results := make([]panelistPromptResult, len(prepared))
	var wg sync.WaitGroup
	for i := range prepared {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i] = svc.runR3PanelistPrompt(&briefSnapshot, room.SummaryR2, r2Items, prepared[i], cwds[i])
		}(i)
	}
	wg.Wait()

	// Persist speech turns + seat status (and re-persist acp_session_id).
	speechTurns := make([]Turn, 0, len(results))
	r3Items := make([]r2SpeechItem, 0, len(results))
	var failedRoles []string
	for _, r := range results {
		seat := r.seat
		if r.err != nil {
			seat.Status = SeatFailed
			// Keep prior acp_session_id when resume failed mid-flight.
			if r.acpID != "" {
				seat.AcpSessionID = r.acpID
			}
			_ = svc.store.UpdateSeatSession(&seat)

			failMsg := fmt.Sprintf("[failed] %s 席 R3 发言失败：%v", RoleLabel(seat.Role), r.err)
			turn := Turn{
				ID:          meta.NewID(),
				RoomID:      roomID,
				Round:       3,
				SeatID:      seat.ID,
				Kind:        TurnKindSpeech,
				ContentText: failMsg,
				ProcessRef:  seat.SessionID,
				CreatedAt:   time.Now().UTC(),
			}
			if err := svc.store.InsertTurn(&turn); err != nil {
				return nil, err
			}
			if run != nil {
				if err := svc.store.FinishRunSeat(run.ID, seat.Role, true, r.err.Error()); err != nil {
					return nil, err
				}
			}
			speechTurns = append(speechTurns, turn)
			r3Items = append(r3Items, r2SpeechItem{
				Role: seat.Role, SeatID: seat.ID, Failed: true, FailMsg: failMsg,
			})
			failedRoles = append(failedRoles, string(seat.Role))
			log.Printf("[roundtable] R3 seat failed room=%s role=%s err=%v", roomID, seat.Role, r.err)
			continue
		}

		// Persist same acp across R2→R3 (must not mint a new id on success path).
		if r.acpID != "" {
			seat.AcpSessionID = r.acpID
			if svc.chatStore != nil && seat.SessionID != "" {
				_ = svc.chatStore.UpdateACP(seat.SessionID, r.acpID)
			}
		}
		seat.Status = SeatDone
		if err := svc.store.UpdateSeatSession(&seat); err != nil {
			return nil, err
		}

		text := strings.TrimSpace(r.text)
		if text == "" {
			text = fmt.Sprintf("（%s 本轮无正文输出）", RoleLabel(seat.Role))
		}
		turn := Turn{
			ID:          meta.NewID(),
			RoomID:      roomID,
			Round:       3,
			SeatID:      seat.ID,
			Kind:        TurnKindSpeech,
			ContentText: text,
			ProcessRef:  seat.SessionID,
			CreatedAt:   time.Now().UTC(),
		}
		if err := svc.store.InsertTurn(&turn); err != nil {
			return nil, err
		}

		// === 立即写回结论：对话结束就拉最新回复持久化 ===
		if run != nil {
			if err := svc.store.FinishRunSeat(run.ID, seat.Role, false, ""); err != nil {
				return nil, err
			}
		}
		speechTurns = append(speechTurns, turn)
		r3Items = append(r3Items, r2SpeechItem{
			Role: seat.Role, SeatID: seat.ID, Text: text,
		})
	}

	if run != nil && len(failedRoles) > 0 {
		if err := svc.store.PauseRoundRunForSeats(run.ID); err != nil {
			return nil, err
		}
		full, err := svc.GetRoom(roomID)
		if err != nil {
			return nil, err
		}
		return &RunR3Response{
			Room:        full,
			SpeechTurns: speechTurns,
			FailedRoles: failedRoles,
		}, nil
	}

	// --- summarizing_r3 ---
	if run == nil {
		if err := Transition(room, StateSummarizingR3); err != nil {
			return nil, err
		}
		if err := svc.store.UpdateRoomState(room); err != nil {
			return nil, err
		}
	} else if err := svc.store.UpdateRunStatus(run.ID, RunSummarizing, ""); err != nil {
		return nil, err
	}

	// --- Referee Summary₃ (终稿) ---
	summaryTurn, summaryText, err := svc.runR3RefereeSummary(room, &briefSnapshot, referee, r2Items, r3Items)
	if err != nil {
		if run == nil {
			_ = Transition(room, StateFailed)
			_ = svc.store.UpdateRoomState(room)
		}
		return nil, fmt.Errorf("r3 summary: %w", err)
	}

	room.SummaryR3 = summaryText
	if run == nil {
		if err := Transition(room, StateDone); err != nil {
			return nil, err
		}
		if err := svc.store.UpdateRoomSummaryR3AndState(room); err != nil {
			return nil, err
		}
	} else {
		status := RunCompleted
		if len(failedRoles) > 0 {
			status = RunPartialFailed
		}
		if err := svc.store.FinalizeRoundRun(
			run.ID,
			status,
			StateDone,
			summaryText,
			"",
			RunErrorNone,
		); err != nil {
			return nil, err
		}
	}

	full, err := svc.GetRoom(roomID)
	if err != nil {
		return nil, err
	}
	return &RunR3Response{
		Room:        full,
		SpeechTurns: speechTurns,
		SummaryTurn: summaryTurn,
		FailedRoles: failedRoles,
	}, nil
}

// runR3PanelistPrompt resumes the seat's R2 acp_session_id and injects the
// public context package. Safe for concurrent calls (no store writes).
func (svc *Service) runR3PanelistPrompt(brief *Brief, summaryR2 string, r2Items []r2SpeechItem, seat Seat, cwd string) panelistPromptResult {
	out := panelistPromptResult{seat: seat}

	// Hard rule: R3 must resume the same ACP session created in R2.
	if strings.TrimSpace(seat.AcpSessionID) == "" {
		out.err = fmt.Errorf("missing acp_session_id for resume (R2 did not establish session)")
		return out
	}

	promptText := BuildR3PanelistPrompt(seat.Role, brief, r2Items, summaryR2)
	out.injectText = promptText
	// Resume path: SystemContext intentionally empty (private R2 history retained by ACP).
	out.injectSys = ""

	log.Printf("[roundtable] R3 resume acp=%s role=%s session=%s text_len=%d public_ctx=brief+speech2+summary2 tool_trace=forbidden",
		seat.AcpSessionID, seat.Role, seat.SessionID, len(promptText))

	result, err := svc.prompter.Prompt(SeatPromptRequest{
		SessionID:      seat.SessionID,
		WorkspacePath:  cwd,
		AgentType:      seat.AgentType,
		AcpSessionID:   seat.AcpSessionID, // same id as R2
		Text:           promptText,
		SystemContext:  "", // resume: do not re-seed system; public package is in Text
		PermissionMode: "approve-all",
		Role:           seat.Role,
	})
	if err != nil {
		out.err = err
		// Preserve requested resume id for diagnostics.
		out.acpID = seat.AcpSessionID
		return out
	}
	out.text = result.Text
	// Prefer bridge-returned id; fall back to the resume id we sent.
	out.acpID = result.AcpSessionID
	if out.acpID == "" {
		out.acpID = seat.AcpSessionID
	}
	return out
}

// runR3RefereeSummary resumes the referee session and produces Summary₃ 终稿.
func (svc *Service) runR3RefereeSummary(room *Room, brief *Brief, referee *Seat, r2Items, r3Items []r2SpeechItem) (Turn, string, error) {
	if strings.TrimSpace(referee.SessionID) == "" {
		cwd, err := svc.resolveCwd(referee.WorkspaceID)
		if err != nil {
			return Turn{}, "", fmt.Errorf("resolve referee cwd: %w", err)
		}
		if err := svc.indexSeatSession(referee, room.Title, cwd); err != nil {
			return Turn{}, "", err
		}
		if err := svc.store.UpdateSeatSession(referee); err != nil {
			return Turn{}, "", err
		}
	}
	cwd, err := svc.resolveCwd(referee.WorkspaceID)
	if err != nil {
		return Turn{}, "", fmt.Errorf("resolve referee cwd: %w", err)
	}

	referee.Status = SeatSpeaking
	_ = svc.store.UpdateSeatSession(referee)

	promptText := BuildR3SummaryPrompt(brief, r2Items, room.SummaryR2, r3Items)
	sysCtx := ""
	if referee.AcpSessionID == "" {
		sysCtx = "当前阶段：R3 末裁判终稿。只输出 Summary₃ 正文。"
	}
	result, err := svc.prompter.Prompt(SeatPromptRequest{
		SessionID:      referee.SessionID,
		WorkspacePath:  cwd,
		AgentType:      referee.AgentType,
		AcpSessionID:   referee.AcpSessionID,
		Text:           promptText,
		SystemContext:  sysCtx,
		PermissionMode: "approve-all",
		Role:           RoleReferee,
	})
	if err != nil {
		referee.Status = SeatReady
		_ = svc.store.UpdateSeatSession(referee)
		return Turn{}, "", err
	}
	if result.AcpSessionID != "" {
		referee.AcpSessionID = result.AcpSessionID
		if svc.chatStore != nil && referee.SessionID != "" {
			_ = svc.chatStore.UpdateACP(referee.SessionID, result.AcpSessionID)
		}
	}
	referee.Status = SeatReady
	if err := svc.store.UpdateSeatSession(referee); err != nil {
		return Turn{}, "", err
	}

	summaryText := strings.TrimSpace(result.Text)
	if summaryText == "" {
		summaryText = "（裁判 Summary₃ 无正文输出）"
	}
	summaryText = appendMissingRoleRecord(summaryText, r3Items)

	turn := Turn{
		ID:          meta.NewID(),
		RoomID:      room.ID,
		Round:       3,
		SeatID:      referee.ID,
		Kind:        TurnKindSummary,
		ContentText: summaryText,
		ProcessRef:  referee.SessionID,
		CreatedAt:   time.Now().UTC(),
	}
	if err := svc.store.InsertTurn(&turn); err != nil {
		return Turn{}, "", err
	}

	room.SummaryR3 = summaryText
	return turn, summaryText, nil
}
