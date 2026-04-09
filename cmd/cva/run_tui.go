package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/siisee11/CodexVirtualAssistant/internal/assistant"
)

var (
	tuiHeaderStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("63")).
			Padding(0, 1)
	tuiActivityStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("240")).
				Padding(0, 1)
	tuiComposerStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("69")).
				Padding(0, 1)
	tuiSectionTitleStyle = lipgloss.NewStyle().
				Bold(true)
	tuiMutedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("244"))
)

type runTUIClient interface {
	CreateRun(ctx context.Context, request string, maxAttempts int, parentRunID string) (*createRunResponse, error)
	ResumeRun(ctx context.Context, runID string, input map[string]string) error
	StreamEvents(ctx context.Context, runID string) (io.ReadCloser, error)
}

type tuiContextDoneMsg struct{}

type tuiStreamReadyMsg struct {
	streamID   int
	runID      string
	streamMsgs <-chan tea.Msg
	cancel     context.CancelFunc
	err        error
}

type tuiStreamEventMsg struct {
	streamID int
	event    assistant.RunEvent
}

type tuiStreamErrMsg struct {
	streamID int
	err      error
}

type tuiStreamClosedMsg struct {
	streamID int
}

type tuiComposerSubmitDoneMsg struct {
	notice       string
	err          error
	nextRun      *assistant.Run
	streamID     int
	streamMsgs   <-chan tea.Msg
	streamCancel context.CancelFunc
}

type composerMode int

const (
	composerModeLocked composerMode = iota
	composerModeWaiting
	composerModeFollowUp
	composerModeSubmitting
)

type runTUIModel struct {
	ctx    context.Context
	client runTUIClient
	run    assistant.Run

	width  int
	height int
	ready  bool

	status assistant.RunStatus
	phase  assistant.RunPhase

	viewport         viewport.Model
	composer         textarea.Model
	activityLines    []string
	streamMsgs       <-chan tea.Msg
	streamClosed     bool
	streamErr        error
	streamCancel     context.CancelFunc
	activeStreamID   int
	nextStreamID     int
	followLogs       bool
	lastPhaseSummary string
	lastPhaseAt      time.Time
	waitingSummary   string
	submitting       bool
}

func runRunTUI(ctx context.Context, client runTUIClient, run assistant.Run) error {
	program := tea.NewProgram(
		newRunTUIModel(ctx, client, run),
		tea.WithAltScreen(),
		tea.WithContext(ctx),
	)
	_, err := program.Run()
	return err
}

func newRunTUIModel(ctx context.Context, client runTUIClient, run assistant.Run) runTUIModel {
	composer := textarea.New()
	composer.Placeholder = "Provide input when prompted or after completion for follow-up"
	composer.Prompt = "> "
	composer.ShowLineNumbers = false
	composer.SetHeight(3)
	composer.Focus()

	model := runTUIModel{
		ctx:              ctx,
		client:           client,
		run:              run,
		status:           run.Status,
		phase:            run.Phase,
		viewport:         viewport.New(0, 0),
		composer:         composer,
		followLogs:       true,
		lastPhaseSummary: "Run created from the user request.",
		lastPhaseAt:      run.CreatedAt,
		activeStreamID:   1,
		nextStreamID:     2,
		activityLines: []string{
			fmt.Sprintf("Run created: %s", run.ID),
			fmt.Sprintf("Chat: %s", run.ChatID),
			"Connecting to run event stream.",
			"Press q or Ctrl+C to quit.",
		},
	}
	model.viewport.SetContent(strings.Join(model.activityLines, "\n"))
	model.viewport.GotoBottom()

	return model
}

func (m runTUIModel) Init() tea.Cmd {
	return tea.Batch(
		textarea.Blink,
		waitForTUIContextDone(m.ctx),
		openRunStreamCmd(m.ctx, m.client, m.run.ID, m.activeStreamID),
	)
}

func (m runTUIModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tuiContextDoneMsg:
		return m, tea.Quit
	case tuiStreamReadyMsg:
		if msg.streamID != m.activeStreamID {
			if msg.cancel != nil {
				msg.cancel()
			}
			return m, nil
		}
		if msg.err != nil {
			m.streamErr = msg.err
			m.streamClosed = true
			m.addActivityLine(fmt.Sprintf("Stream error: %v", msg.err))
			return m, nil
		}
		if m.streamCancel != nil {
			m.streamCancel()
		}
		m.streamCancel = msg.cancel
		m.streamMsgs = msg.streamMsgs
		m.streamClosed = false
		m.streamErr = nil
		m.addActivityLine(fmt.Sprintf("Streaming events for %s.", msg.runID))
		return m, waitForTUIStreamMsg(m.streamMsgs, msg.streamID)
	case tuiStreamEventMsg:
		if msg.streamID != m.activeStreamID {
			return m, nil
		}
		m.handleRunEvent(msg.event)
		return m, waitForTUIStreamMsg(m.streamMsgs, msg.streamID)
	case tuiStreamErrMsg:
		if msg.streamID != m.activeStreamID {
			return m, nil
		}
		m.streamErr = msg.err
		m.addActivityLine(fmt.Sprintf("Stream error: %v", msg.err))
		return m, waitForTUIStreamMsg(m.streamMsgs, msg.streamID)
	case tuiStreamClosedMsg:
		if msg.streamID != m.activeStreamID {
			return m, nil
		}
		m.streamClosed = true
		if m.streamErr == nil {
			m.addActivityLine("Event stream closed.")
		}
		return m, nil
	case tuiComposerSubmitDoneMsg:
		m.submitting = false
		m.composer.SetValue("")
		if msg.err != nil {
			m.addActivityLine(fmt.Sprintf("Submit failed: %v", msg.err))
			return m, nil
		}
		if msg.nextRun != nil {
			m.run = *msg.nextRun
			m.status = m.run.Status
			m.phase = m.run.Phase
			m.waitingSummary = ""
			m.lastPhaseSummary = firstNonEmptyTUI(msg.notice, fmt.Sprintf("Run %s started.", m.run.ID))
			m.lastPhaseAt = time.Now().UTC()
		}
		if msg.notice != "" {
			m.addActivityLine(msg.notice)
		}
		if msg.streamMsgs != nil {
			m.streamMsgs = msg.streamMsgs
			m.streamCancel = msg.streamCancel
			m.streamClosed = false
			m.streamErr = nil
			m.activeStreamID = msg.streamID
			return m, waitForTUIStreamMsg(m.streamMsgs, msg.streamID)
		}
		return m, nil
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.ready = true
		m.syncLayout()
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			if m.streamCancel != nil {
				m.streamCancel()
			}
			return m, tea.Quit
		case "enter":
			mode := m.currentComposerMode()
			if mode == composerModeLocked || mode == composerModeSubmitting {
				return m, nil
			}
			input := strings.TrimSpace(m.composer.Value())
			if input == "" {
				m.addActivityLine("Composer input is empty.")
				return m, nil
			}
			m.submitting = true
			m.followLogs = true
			streamID := m.nextStreamID
			m.nextStreamID++
			m.activeStreamID = streamID
			if m.streamCancel != nil {
				m.streamCancel()
				m.streamCancel = nil
			}
			m.streamMsgs = nil
			m.streamClosed = true
			m.streamErr = nil
			m.addActivityLine("Submitting composer input...")
			return m, submitComposerCmd(m.ctx, m.client, mode, m.run, input, streamID)
		case "pgup":
			m.viewport.HalfViewUp()
			m.followLogs = false
			return m, nil
		case "pgdown":
			m.viewport.HalfViewDown()
			m.followLogs = false
			return m, nil
		case "home":
			m.viewport.GotoTop()
			m.followLogs = false
			return m, nil
		case "end":
			m.viewport.GotoBottom()
			m.followLogs = true
			return m, nil
		case "up", "k":
			m.followLogs = false
		case "down", "j":
			m.followLogs = false
		case "f":
			m.followLogs = !m.followLogs
			if m.followLogs {
				m.viewport.GotoBottom()
			}
			return m, nil
		}
	}

	var cmds []tea.Cmd
	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	cmds = append(cmds, cmd)
	if mode := m.currentComposerMode(); mode == composerModeWaiting || mode == composerModeFollowUp {
		m.composer, cmd = m.composer.Update(msg)
		cmds = append(cmds, cmd)
	}
	return m, tea.Batch(cmds...)
}

func (m runTUIModel) View() string {
	if !m.ready {
		return "Initializing full-screen run view..."
	}

	header := m.renderHeader()
	activity := m.renderActivity()
	composer := m.renderComposer()

	return lipgloss.JoinVertical(lipgloss.Left, header, activity, composer)
}

func (m *runTUIModel) syncLayout() {
	if !m.ready {
		return
	}

	composerInputWidth := max(10, m.width-6)
	m.composer.SetWidth(composerInputWidth)
	m.composer.SetHeight(3)

	headerHeight := lipgloss.Height(m.renderHeader())
	composerHeight := lipgloss.Height(m.renderComposer())
	activityOuterHeight := m.height - headerHeight - composerHeight
	if activityOuterHeight < 3 {
		activityOuterHeight = 3
	}

	m.viewport.Width = max(1, m.width-4)
	m.viewport.Height = max(1, activityOuterHeight-2)
	m.viewport.SetContent(strings.Join(m.activityLines, "\n"))
}

func (m runTUIModel) renderHeader() string {
	title := tuiSectionTitleStyle.Render("cva run")
	ids := fmt.Sprintf("Run: %s   Chat: %s", m.run.ID, m.run.ChatID)
	status := fmt.Sprintf("Status: %s   Phase: %s   Attempts: %d", m.status, m.phase, m.run.AttemptCount)
	phaseDetail := fmt.Sprintf("Phase detail: %s", m.phaseDetail())
	waitingDetail := ""
	if m.waitingSummary != "" {
		waitingDetail = fmt.Sprintf("Waiting: %s", m.waitingSummary)
	}
	request := fmt.Sprintf("Request: %s", truncateForTUI(singleLine(m.run.UserRequestRaw), max(8, m.width-16)))

	lines := []string{title, ids, status, phaseDetail}
	if waitingDetail != "" {
		lines = append(lines, waitingDetail)
	}
	lines = append(lines, request)
	body := strings.Join(lines, "\n")
	return tuiHeaderStyle.
		Width(max(1, m.width)).
		Render(body)
}

func (m runTUIModel) renderActivity() string {
	label := tuiSectionTitleStyle.Render("Activity")
	parts := []string{label}
	if m.streamErr != nil {
		parts = append(parts, tuiMutedStyle.Render(fmt.Sprintf("Stream state: error (%v)", m.streamErr)))
	} else if m.streamClosed {
		parts = append(parts, tuiMutedStyle.Render("Stream state: closed"))
	} else {
		parts = append(parts, tuiMutedStyle.Render("Stream state: live"))
	}
	followState := "on"
	if !m.followLogs {
		followState = "off"
	}
	parts = append(parts, tuiMutedStyle.Render(fmt.Sprintf("Scroll: PgUp/PgDn/Home/End   Follow: %s (f to toggle)", followState)))
	parts = append(parts, m.viewport.View())
	body := strings.Join(parts, "\n")
	return tuiActivityStyle.
		Width(max(1, m.width)).
		Height(max(3, m.height-lipgloss.Height(m.renderHeader())-lipgloss.Height(m.renderComposer()))).
		Render(body)
}

func (m runTUIModel) renderComposer() string {
	mode := m.currentComposerMode()
	label := tuiSectionTitleStyle.Render("Composer")
	status := tuiMutedStyle.Render(m.composerModeLabel(mode))
	help := tuiMutedStyle.Render(m.composerModeHelp(mode))

	inputView := m.composer.View()
	if mode == composerModeLocked || mode == composerModeSubmitting {
		inputView = tuiMutedStyle.Render(m.composerModePrompt(mode))
	}

	body := strings.Join([]string{label, status, inputView, help}, "\n")
	return tuiComposerStyle.
		Width(max(1, m.width)).
		Render(body)
}

func waitForTUIContextDone(ctx context.Context) tea.Cmd {
	return func() tea.Msg {
		if ctx == nil {
			return nil
		}
		<-ctx.Done()
		return tuiContextDoneMsg{}
	}
}

func waitForTUIStreamMsg(streamMsgs <-chan tea.Msg, streamID int) tea.Cmd {
	return func() tea.Msg {
		if streamMsgs == nil {
			return tuiStreamClosedMsg{streamID: streamID}
		}
		msg, ok := <-streamMsgs
		if !ok {
			return tuiStreamClosedMsg{streamID: streamID}
		}
		return msg
	}
}

func openRunStreamCmd(ctx context.Context, client runTUIClient, runID string, streamID int) tea.Cmd {
	return func() tea.Msg {
		return openRunStream(ctx, client, runID, streamID)
	}
}

func openRunStream(ctx context.Context, client runTUIClient, runID string, streamID int) tuiStreamReadyMsg {
	if client == nil {
		return tuiStreamReadyMsg{streamID: streamID, runID: runID, err: errors.New("missing TUI client")}
	}
	streamCtx, cancel := context.WithCancel(ctx)
	stream, err := client.StreamEvents(streamCtx, runID)
	if err != nil {
		cancel()
		return tuiStreamReadyMsg{streamID: streamID, runID: runID, err: err}
	}
	return tuiStreamReadyMsg{
		streamID:   streamID,
		runID:      runID,
		streamMsgs: streamRunEventsForTUI(streamCtx, stream, streamID),
		cancel:     cancel,
	}
}

func streamRunEventsForTUI(ctx context.Context, stream io.ReadCloser, streamID int) <-chan tea.Msg {
	msgs := make(chan tea.Msg, 32)
	go func() {
		defer close(msgs)
		defer stream.Close()

		err := streamSSE(stream, func(ev assistant.RunEvent) bool {
			select {
			case msgs <- tuiStreamEventMsg{streamID: streamID, event: ev}:
			case <-ctx.Done():
				return false
			}
			return !isTerminalPhase(ev.Phase)
		})
		if err != nil && !errors.Is(ctx.Err(), context.Canceled) {
			select {
			case msgs <- tuiStreamErrMsg{streamID: streamID, err: err}:
			case <-ctx.Done():
				return
			}
		}
	}()
	return msgs
}

func submitComposerCmd(ctx context.Context, client runTUIClient, mode composerMode, run assistant.Run, input string, streamID int) tea.Cmd {
	return func() tea.Msg {
		if client == nil {
			return tuiComposerSubmitDoneMsg{err: errors.New("missing TUI client")}
		}
		switch mode {
		case composerModeWaiting:
			if err := client.ResumeRun(ctx, run.ID, parseResumeInputFromComposer(input)); err != nil {
				return tuiComposerSubmitDoneMsg{err: fmt.Errorf("resume run: %w", err)}
			}
			ready := openRunStream(ctx, client, run.ID, streamID)
			if ready.err != nil {
				return tuiComposerSubmitDoneMsg{err: fmt.Errorf("resume run stream: %w", ready.err)}
			}
			nextRun := run
			return tuiComposerSubmitDoneMsg{
				notice:       fmt.Sprintf("Resumed run %s.", run.ID),
				nextRun:      &nextRun,
				streamID:     streamID,
				streamMsgs:   ready.streamMsgs,
				streamCancel: ready.cancel,
			}
		case composerModeFollowUp:
			created, err := client.CreateRun(ctx, input, 0, run.ID)
			if err != nil {
				return tuiComposerSubmitDoneMsg{err: fmt.Errorf("create follow-up run: %w", err)}
			}
			ready := openRunStream(ctx, client, created.Run.ID, streamID)
			if ready.err != nil {
				return tuiComposerSubmitDoneMsg{err: fmt.Errorf("follow-up run stream: %w", ready.err)}
			}
			nextRun := created.Run
			return tuiComposerSubmitDoneMsg{
				notice:       fmt.Sprintf("Started follow-up run %s.", created.Run.ID),
				nextRun:      &nextRun,
				streamID:     streamID,
				streamMsgs:   ready.streamMsgs,
				streamCancel: ready.cancel,
			}
		default:
			return tuiComposerSubmitDoneMsg{notice: "Composer is not available in the current run state."}
		}
	}
}

func parseResumeInputFromComposer(input string) map[string]string {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return map[string]string{}
	}
	fields := strings.Fields(trimmed)
	allKeyValues := len(fields) > 0
	result := make(map[string]string, len(fields))
	for _, field := range fields {
		parts := strings.SplitN(field, "=", 2)
		if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" {
			allKeyValues = false
			break
		}
		result[parts[0]] = parts[1]
	}
	if allKeyValues {
		return result
	}
	return map[string]string{"response": trimmed}
}

func (m *runTUIModel) handleRunEvent(ev assistant.RunEvent) {
	m.addActivityLine(formatEvent(ev))
	if ev.Phase != "" {
		if ev.Phase != m.phase {
			m.lastPhaseSummary = firstNonEmptyTUI(ev.Summary, fmt.Sprintf("Phase changed to %s.", ev.Phase))
			m.lastPhaseAt = ev.CreatedAt
		}
		m.phase = ev.Phase
		m.status = runStatusForPhase(ev.Phase, m.status)
	}
	if ev.Type == assistant.EventTypeWaiting {
		m.status = assistant.RunStatusWaiting
		m.phase = assistant.RunPhaseWaiting
		m.waitingSummary = firstNonEmptyTUI(ev.Summary, "Run is waiting for user input.")
		m.lastPhaseSummary = m.waitingSummary
		m.lastPhaseAt = ev.CreatedAt
	}
	if ev.Type == assistant.EventTypePhaseChanged {
		if ev.Phase == assistant.RunPhaseCompleted ||
			ev.Phase == assistant.RunPhaseFailed ||
			ev.Phase == assistant.RunPhaseCancelled {
			m.followLogs = true
		}
		if ev.Phase != assistant.RunPhaseWaiting {
			m.waitingSummary = ""
		}
	}
}

func (m *runTUIModel) addActivityLine(line string) {
	prevY := m.viewport.YOffset
	m.activityLines = append(m.activityLines, line)
	m.viewport.SetContent(strings.Join(m.activityLines, "\n"))
	if m.followLogs {
		m.viewport.GotoBottom()
		return
	}
	maxOffset := max(0, m.viewport.TotalLineCount()-m.viewport.Height)
	if prevY > maxOffset {
		prevY = maxOffset
	}
	m.viewport.YOffset = prevY
}

func (m runTUIModel) currentComposerMode() composerMode {
	if m.submitting {
		return composerModeSubmitting
	}
	if m.phase == assistant.RunPhaseWaiting || m.status == assistant.RunStatusWaiting {
		return composerModeWaiting
	}
	if isTerminalPhase(m.phase) {
		return composerModeFollowUp
	}
	return composerModeLocked
}

func (m runTUIModel) composerModeLabel(mode composerMode) string {
	switch mode {
	case composerModeWaiting:
		return "Mode: waiting input"
	case composerModeFollowUp:
		return "Mode: follow-up"
	case composerModeSubmitting:
		return "Mode: submitting"
	default:
		return "Mode: read-only"
	}
}

func (m runTUIModel) composerModePrompt(mode composerMode) string {
	switch mode {
	case composerModeSubmitting:
		return "Submitting..."
	case composerModeLocked:
		return "Initial request already submitted. Composer activates when waiting or after completion."
	default:
		return ""
	}
}

func (m runTUIModel) composerModeHelp(mode composerMode) string {
	switch mode {
	case composerModeWaiting:
		return "Enter to resume this run. Accepts key=value pairs or free text."
	case composerModeFollowUp:
		return "Enter to start a follow-up run in the same chat."
	case composerModeSubmitting:
		return "Submitting request..."
	default:
		return "Composer is locked while the run is actively streaming."
	}
}

func runStatusForPhase(phase assistant.RunPhase, fallback assistant.RunStatus) assistant.RunStatus {
	switch phase {
	case assistant.RunPhaseQueued:
		return assistant.RunStatusQueued
	case assistant.RunPhaseGating:
		return assistant.RunStatusGating
	case assistant.RunPhaseAnswering:
		return assistant.RunStatusAnswering
	case assistant.RunPhaseSelectingProject:
		return assistant.RunStatusSelectingProject
	case assistant.RunPhasePlanning:
		return assistant.RunStatusPlanning
	case assistant.RunPhaseContracting:
		return assistant.RunStatusContracting
	case assistant.RunPhaseGenerating:
		return assistant.RunStatusGenerating
	case assistant.RunPhaseEvaluating:
		return assistant.RunStatusEvaluating
	case assistant.RunPhaseScheduling:
		return assistant.RunStatusScheduling
	case assistant.RunPhaseWikiIngesting:
		return assistant.RunStatusWikiIngesting
	case assistant.RunPhaseReporting:
		return assistant.RunStatusReporting
	case assistant.RunPhaseWaiting:
		return assistant.RunStatusWaiting
	case assistant.RunPhaseCompleted:
		return assistant.RunStatusCompleted
	case assistant.RunPhaseFailed:
		return assistant.RunStatusFailed
	case assistant.RunPhaseCancelled:
		return assistant.RunStatusCancelled
	default:
		return fallback
	}
}

func (m runTUIModel) phaseDetail() string {
	summary := firstNonEmptyTUI(m.lastPhaseSummary, "Waiting for updates.")
	if m.lastPhaseAt.IsZero() {
		return summary
	}
	return fmt.Sprintf("%s (%s)", summary, m.lastPhaseAt.Local().Format("15:04:05"))
}

func truncateForTUI(value string, maxLen int) string {
	if maxLen <= 0 || len(value) <= maxLen {
		return value
	}
	if maxLen <= 3 {
		return value[:maxLen]
	}
	return value[:maxLen-3] + "..."
}

func singleLine(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func firstNonEmptyTUI(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
