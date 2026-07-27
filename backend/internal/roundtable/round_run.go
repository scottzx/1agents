package roundtable

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/scottzx/1Agents/backend/internal/meta"
)

// StartRoundResponse is returned immediately by the asynchronous start API.
type StartRoundResponse struct {
	RunID  string    `json:"run_id"`
	Run    *RoundRun `json:"run"`
	Room   *Room     `json:"room"`
	Reused bool      `json:"reused"`
}

func (svc *Service) StartR2(roomID, idempotencyKey string) (*StartRoundResponse, error) {
	return svc.startRound(roomID, 2, idempotencyKey)
}

func (svc *Service) StartR3(roomID, idempotencyKey string) (*StartRoundResponse, error) {
	return svc.startRound(roomID, 3, idempotencyKey)
}

func (svc *Service) startRound(roomID string, round int, idempotencyKey string) (*StartRoundResponse, error) {
	if svc == nil || svc.store == nil {
		return nil, fmt.Errorf("roundtable: service not configured")
	}
	run, created, err := svc.store.ClaimRoundRun(roomID, round, idempotencyKey)
	if err != nil {
		return nil, err
	}
	if created {
		go svc.executeRoundRun(run)
	}
	room, err := svc.GetRoom(roomID)
	if err != nil {
		return nil, err
	}
	currentRun, err := svc.store.GetRoundRun(run.ID)
	if err != nil {
		return nil, err
	}
	return &StartRoundResponse{
		RunID:  run.ID,
		Run:    currentRun,
		Room:   room,
		Reused: !created,
	}, nil
}

func (svc *Service) executeRoundRun(run *RoundRun) {
	if err := svc.store.UpdateRunStatus(run.ID, RunRunning, ""); err != nil {
		log.Printf("[roundtable] start run failed run=%s err=%v", run.ID, err)
		_ = svc.failRoundRun(run, err)
		return
	}

	var err error
	switch run.Round {
	case 2:
		_, err = svc.runR2(run.RoomID, run)
	case 3:
		_, err = svc.runR3(run.RoomID, run)
	default:
		err = fmt.Errorf("roundtable: unsupported round %d", run.Round)
	}
	if err != nil {
		log.Printf("[roundtable] asynchronous run failed run=%s round=%d err=%v", run.ID, run.Round, err)
		_ = svc.failRoundRun(run, err)
	}
}

func (svc *Service) failRoundRun(run *RoundRun, runErr error) error {
	current, err := svc.store.GetRoundRun(run.ID)
	if err != nil {
		return err
	}
	if isTerminalRunStatus(current.Status) {
		return nil
	}
	room, err := svc.store.GetRoom(run.RoomID)
	if err != nil {
		return err
	}
	summary := room.SummaryR2
	if run.Round == 3 {
		summary = room.SummaryR3
	}
	errorScope := RunErrorRoom
	if current.Status == RunSummarizing {
		errorScope = RunErrorSummary
	}
	return svc.store.FinalizeRoundRun(
		run.ID,
		RunFailed,
		StateFailed,
		summary,
		runErr.Error(),
		errorScope,
	)
}

// WaitRoundRun is used only by the explicit legacy ?wait=1 HTTP compatibility
// path. New clients should consume room/events and never hold this long request.
func (svc *Service) WaitRoundRun(ctx context.Context, runID string) (*RoundRun, error) {
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		run, err := svc.store.GetRoundRun(runID)
		if err != nil {
			return nil, err
		}
		if isTerminalRunStatus(run.Status) {
			return run, nil
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
		}
	}
}

func (svc *Service) GetRoundRun(runID string) (*RoundRun, error) {
	if svc == nil || svc.store == nil {
		return nil, fmt.Errorf("roundtable: service not configured")
	}
	return svc.store.GetRoundRun(runID)
}

func (svc *Service) ListRoundEvents(roomID string, after int64, limit int) ([]RoundEvent, error) {
	if svc == nil || svc.store == nil {
		return nil, fmt.Errorf("roundtable: service not configured")
	}
	if _, err := svc.store.GetRoom(roomID); err != nil {
		return nil, err
	}
	return svc.store.ListRoundEvents(roomID, after, limit)
}

// RecoverRoundResponse is returned by retry/skip recovery actions. Every action
// continues the same durable RoundRun instead of creating a replacement run.
type RecoverRoundResponse struct {
	RunID  string    `json:"run_id"`
	Run    *RoundRun `json:"run"`
	Room   *Room     `json:"room"`
	Reused bool      `json:"reused"`
}

func (svc *Service) RetryRoundSeat(roomID, runID string, role Role) (*RecoverRoundResponse, error) {
	if !IsPanelist(role) {
		return nil, fmt.Errorf("roundtable: invalid panelist role %s", role)
	}
	run, err := svc.store.GetRoundRun(runID)
	if err != nil {
		return nil, err
	}
	if run.RoomID != roomID {
		return nil, fmt.Errorf("roundtable: run does not belong to room")
	}
	claimed, err := svc.store.ClaimFailedSeatRetry(runID, role)
	if err != nil {
		return nil, err
	}
	if claimed {
		go svc.executeRoundSeatRetry(runID, role)
	}
	return svc.recoveryResponse(roomID, runID, !claimed)
}

func (svc *Service) SkipFailedSeatsAndSummarize(roomID, runID string) (*RecoverRoundResponse, error) {
	run, err := svc.store.GetRoundRun(runID)
	if err != nil {
		return nil, err
	}
	if run.RoomID != roomID {
		return nil, fmt.Errorf("roundtable: run does not belong to room")
	}
	if _, err := svc.store.SkipFailedSeats(runID); err != nil {
		current, getErr := svc.store.GetRoundRun(runID)
		if getErr == nil {
			progress, progressErr := svc.store.RunProgress(runID)
			if progressErr == nil && len(progress.SkippedRoles) > 0 &&
				(current.Status == RunSummarizing || current.Status == RunCompleted ||
					(current.Status == RunPartialFailed && current.ErrorScope == RunErrorNone)) {
				return svc.recoveryResponse(roomID, runID, true)
			}
		}
		return nil, err
	}
	go svc.summarizeRoundRun(runID)
	return svc.recoveryResponse(roomID, runID, false)
}

func (svc *Service) RetryRoundSummary(roomID, runID string) (*RecoverRoundResponse, error) {
	run, err := svc.store.GetRoundRun(runID)
	if err != nil {
		return nil, err
	}
	if run.RoomID != roomID {
		return nil, fmt.Errorf("roundtable: run does not belong to room")
	}
	claimed, err := svc.store.ClaimSummaryRetry(runID)
	if err != nil {
		return nil, err
	}
	if claimed {
		go svc.summarizeRoundRun(runID)
	}
	return svc.recoveryResponse(roomID, runID, !claimed)
}

func (svc *Service) recoveryResponse(roomID, runID string, reused bool) (*RecoverRoundResponse, error) {
	run, err := svc.store.GetRoundRun(runID)
	if err != nil {
		return nil, err
	}
	room, err := svc.GetRoom(roomID)
	if err != nil {
		return nil, err
	}
	return &RecoverRoundResponse{RunID: runID, Run: run, Room: room, Reused: reused}, nil
}

func (svc *Service) executeRoundSeatRetry(runID string, role Role) {
	if err := svc.retryRoundSeat(runID, role); err != nil {
		log.Printf("[roundtable] seat retry failed run=%s role=%s err=%v", runID, role, err)
		_ = svc.finishRetriedSeatFailure(runID, role, err)
	}
}

func (svc *Service) retryRoundSeat(runID string, role Role) error {
	run, err := svc.store.GetRoundRun(runID)
	if err != nil {
		return err
	}
	room, err := svc.store.GetRoom(run.RoomID)
	if err != nil {
		return err
	}
	seats, err := svc.store.ListSeats(run.RoomID)
	if err != nil {
		return err
	}
	var target *Seat
	for i := range seats {
		if seats[i].Role == role {
			target = &seats[i]
			break
		}
	}
	if target == nil {
		return fmt.Errorf("roundtable: retry seat %s missing", role)
	}
	cwd, err := svc.resolveCwd(target.WorkspaceID)
	if err != nil {
		return err
	}
	if target.SessionID == "" {
		if err := svc.indexSeatSession(target, room.Title, cwd); err != nil {
			return err
		}
	}
	target.Status = SeatSpeaking
	if err := svc.store.UpdateSeatSession(target); err != nil {
		return err
	}

	briefVersion, err := svc.store.GetBriefVersion(run.RoomID, room.R2BriefVersion)
	if err != nil {
		return err
	}
	var result panelistPromptResult
	if run.Round == 2 {
		result = svc.runR2PanelistPrompt(&briefVersion.Content, *target, cwd)
	} else {
		r2Items, collectErr := svc.collectR2SpeechItems(run.RoomID, seats)
		if collectErr != nil {
			return collectErr
		}
		result = svc.runR3PanelistPrompt(&briefVersion.Content, room.SummaryR2, r2Items, *target, cwd)
	}
	if result.err != nil {
		return result.err
	}

	target.Status = SeatDone
	if result.acpID != "" {
		target.AcpSessionID = result.acpID
		if svc.chatStore != nil && target.SessionID != "" {
			_ = svc.chatStore.UpdateACP(target.SessionID, result.acpID)
		}
	}
	if err := svc.store.UpdateSeatSession(target); err != nil {
		return err
	}
	text := strings.TrimSpace(result.text)
	if text == "" {
		text = fmt.Sprintf("（%s 本轮无正文输出）", RoleLabel(role))
	}
	turn := Turn{
		ID:          meta.NewID(),
		RoomID:      run.RoomID,
		Round:       run.Round,
		SeatID:      target.ID,
		Kind:        TurnKindSpeech,
		ContentText: text,
		ProcessRef:  target.SessionID,
		CreatedAt:   time.Now().UTC(),
	}
	if err := svc.store.InsertTurn(&turn); err != nil {
		return err
	}
	if err := svc.store.FinishRunSeat(runID, role, false, ""); err != nil {
		return err
	}
	progress, err := svc.store.RunProgress(runID)
	if err != nil {
		return err
	}
	if len(progress.FailedRoles) > 0 {
		return svc.store.PauseRoundRunForSeats(runID)
	}
	if err := svc.store.UpdateRunStatus(runID, RunSummarizing, ""); err != nil {
		return err
	}
	svc.summarizeRoundRun(runID)
	return nil
}

func (svc *Service) finishRetriedSeatFailure(runID string, role Role, retryErr error) error {
	run, err := svc.store.GetRoundRun(runID)
	if err != nil {
		return err
	}
	seats, err := svc.store.ListSeats(run.RoomID)
	if err != nil {
		return err
	}
	for i := range seats {
		if seats[i].Role != role {
			continue
		}
		seats[i].Status = SeatFailed
		_ = svc.store.UpdateSeatSession(&seats[i])
		turn := Turn{
			ID:          meta.NewID(),
			RoomID:      run.RoomID,
			Round:       run.Round,
			SeatID:      seats[i].ID,
			Kind:        TurnKindSpeech,
			ContentText: fmt.Sprintf("[failed] %s 席 R%d 重试失败：%v", RoleLabel(role), run.Round, retryErr),
			ProcessRef:  seats[i].SessionID,
			CreatedAt:   time.Now().UTC(),
		}
		_ = svc.store.InsertTurn(&turn)
		break
	}
	if err := svc.store.FinishRunSeat(runID, role, true, retryErr.Error()); err != nil {
		return err
	}
	return svc.store.PauseRoundRunForSeats(runID)
}

func (svc *Service) summarizeRoundRun(runID string) {
	if err := svc.runStoredSummary(runID); err != nil {
		log.Printf("[roundtable] summary recovery failed run=%s err=%v", runID, err)
		run, getErr := svc.store.GetRoundRun(runID)
		if getErr != nil {
			return
		}
		room, roomErr := svc.store.GetRoom(run.RoomID)
		if roomErr != nil {
			return
		}
		summary := room.SummaryR2
		if run.Round == 3 {
			summary = room.SummaryR3
		}
		_ = svc.store.FinalizeRoundRun(
			runID, RunFailed, StateFailed, summary, err.Error(), RunErrorSummary,
		)
	}
}

func (svc *Service) runStoredSummary(runID string) error {
	run, err := svc.store.GetRoundRun(runID)
	if err != nil {
		return err
	}
	if run.Status != RunSummarizing {
		return fmt.Errorf("roundtable: summary requires summarizing status (status=%s)", run.Status)
	}
	room, err := svc.store.GetRoom(run.RoomID)
	if err != nil {
		return err
	}
	seats, err := svc.store.ListSeats(run.RoomID)
	if err != nil {
		return err
	}
	var referee *Seat
	for i := range seats {
		if seats[i].Role == RoleReferee {
			referee = &seats[i]
			break
		}
	}
	if referee == nil {
		return fmt.Errorf("roundtable: referee seat missing")
	}
	briefVersion, err := svc.store.GetBriefVersion(run.RoomID, room.R2BriefVersion)
	if err != nil {
		return err
	}
	items, err := svc.collectRunSpeechItems(run, seats)
	if err != nil {
		return err
	}
	var summaryText string
	if run.Round == 2 {
		_, summaryText, err = svc.runR2RefereeSummary(room, &briefVersion.Content, referee, items)
	} else {
		var r2Items []r2SpeechItem
		r2Items, err = svc.collectR2SpeechItems(run.RoomID, seats)
		if err == nil {
			_, summaryText, err = svc.runR3RefereeSummary(
				room, &briefVersion.Content, referee, r2Items, items,
			)
		}
	}
	if err != nil {
		return err
	}
	progress, err := svc.store.RunProgress(runID)
	if err != nil {
		return err
	}
	status := RunCompleted
	if len(progress.SkippedRoles) > 0 {
		status = RunPartialFailed
	}
	nextState := StateWaitingR3
	if run.Round == 3 {
		nextState = StateDone
	}
	return svc.store.FinalizeRoundRun(
		runID, status, nextState, summaryText, "", RunErrorNone,
	)
}

func (svc *Service) collectRunSpeechItems(run *RoundRun, seats []Seat) ([]r2SpeechItem, error) {
	turns, err := svc.store.ListTurns(run.RoomID)
	if err != nil {
		return nil, err
	}
	progress, err := svc.store.RunProgress(run.ID)
	if err != nil {
		return nil, err
	}
	skipped := map[Role]bool{}
	for _, value := range progress.SkippedRoles {
		skipped[Role(value)] = true
	}
	seatByID := map[string]Seat{}
	for _, seat := range seats {
		seatByID[seat.ID] = seat
	}
	latest := map[Role]r2SpeechItem{}
	for _, turn := range turns {
		if turn.Round != run.Round || turn.Kind != TurnKindSpeech {
			continue
		}
		seat, ok := seatByID[turn.SeatID]
		if !ok || !IsPanelist(seat.Role) {
			continue
		}
		item := r2SpeechItem{Role: seat.Role, SeatID: seat.ID, Text: turn.ContentText}
		if strings.HasPrefix(turn.ContentText, "[failed]") {
			item.Failed = true
			item.FailMsg = turn.ContentText
		}
		latest[seat.Role] = item
	}
	items := make([]r2SpeechItem, 0, len(PanelistRoles))
	for _, role := range PanelistRoles {
		if skipped[role] {
			items = append(items, r2SpeechItem{
				Role: role, Failed: true,
				FailMsg: fmt.Sprintf("%s席缺席：该席失败后由用户选择跳过并继续总结。", RoleLabel(role)),
			})
			continue
		}
		if item, ok := latest[role]; ok {
			items = append(items, item)
			continue
		}
		items = append(items, r2SpeechItem{
			Role: role, Failed: true,
			FailMsg: fmt.Sprintf("%s席缺席：没有可用发言。", RoleLabel(role)),
		})
	}
	return items, nil
}

func (svc *Service) buildRunR2Response(roomID string) (*RunR2Response, error) {
	room, err := svc.GetRoom(roomID)
	if err != nil {
		return nil, err
	}
	response := &RunR2Response{Room: room, SpeechTurns: []Turn{}}
	for _, turn := range room.Turns {
		if turn.Round != 2 {
			continue
		}
		switch turn.Kind {
		case TurnKindSpeech:
			response.SpeechTurns = append(response.SpeechTurns, turn)
		case TurnKindSummary:
			response.SummaryTurn = turn
		}
	}
	if room.ActiveRun != nil {
		response.FailedRoles = append(response.FailedRoles, room.Progress.FailedRoles...)
	}
	return response, nil
}

func (svc *Service) buildRunR3Response(roomID string) (*RunR3Response, error) {
	room, err := svc.GetRoom(roomID)
	if err != nil {
		return nil, err
	}
	response := &RunR3Response{Room: room, SpeechTurns: []Turn{}}
	for _, turn := range room.Turns {
		if turn.Round != 3 {
			continue
		}
		switch turn.Kind {
		case TurnKindSpeech:
			response.SpeechTurns = append(response.SpeechTurns, turn)
		case TurnKindSummary:
			response.SummaryTurn = turn
		}
	}
	if room.ActiveRun != nil {
		response.FailedRoles = append(response.FailedRoles, room.Progress.FailedRoles...)
	}
	return response, nil
}

func (svc *Service) projectRoomRuntime(room *Room) error {
	if room == nil {
		return nil
	}
	room.Progress = RoundProgress{
		Total:        roundProgressTotal,
		ActiveRoles:  []string{},
		FailedRoles:  []string{},
		SkippedRoles: []string{},
	}
	room.AvailableActions = []string{}
	switch room.State {
	case StateDraftingBrief:
		room.Phase = "r1"
		room.PhaseStatus = "running"
		room.NextAction = "confirm_brief"
		room.AvailableActions = []string{"confirm_brief"}
	case StateWaitingR2:
		room.Phase = "r2"
		room.PhaseStatus = "ready"
		room.NextAction = "start_r2"
		room.AvailableActions = []string{"start_r2"}
	case StateSummarizingR2:
		room.Phase = "r2"
		room.NextAction = "wait"
	case StateWaitingR3:
		room.Phase = "r3"
		room.PhaseStatus = "ready"
		room.NextAction = "start_r3"
		room.AvailableActions = []string{"start_r3"}
	case StateSummarizingR3:
		room.Phase = "r3"
		room.NextAction = "wait"
	case StateDone:
		room.Phase = "done"
		room.PhaseStatus = string(RunCompleted)
		room.NextAction = "none"
	case StateFailed:
		room.Phase = "failed"
		room.PhaseStatus = string(RunFailed)
		room.NextAction = "inspect_failure"
		room.AvailableActions = []string{"reload_room"}
	}

	run, err := svc.store.latestRoundRun(room.ID)
	if err == meta.ErrNotFound {
		return nil
	}
	if err != nil {
		return err
	}
	room.ActiveRun = run
	progress, err := svc.store.RunProgress(run.ID)
	if err != nil {
		return err
	}
	room.Progress = progress
	if room.State == StateSummarizingR2 || room.State == StateSummarizingR3 {
		room.PhaseStatus = string(run.Status)
	}
	if room.State == StateFailed {
		room.Phase = fmt.Sprintf("r%d", run.Round)
		room.PhaseStatus = string(run.Status)
	}
	switch run.ErrorScope {
	case RunErrorSeat:
		room.NextAction = "retry_failed_seats"
		room.AvailableActions = []string{"retry_failed_seats", "skip_and_summarize"}
	case RunErrorSummary:
		room.NextAction = "retry_summary"
		room.AvailableActions = []string{"retry_summary"}
	case RunErrorRoom:
		room.NextAction = "reload_room"
		room.AvailableActions = []string{"reload_room"}
	}
	return nil
}
