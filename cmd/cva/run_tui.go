package main

import (
	"context"
	"fmt"
	"strings"

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

type tuiContextDoneMsg struct{}

type runTUIModel struct {
	ctx context.Context
	run assistant.Run

	width  int
	height int
	ready  bool

	status assistant.RunStatus
	phase  assistant.RunPhase

	viewport      viewport.Model
	composer      textarea.Model
	activityLines []string
}

func runRunTUI(ctx context.Context, run assistant.Run) error {
	program := tea.NewProgram(
		newRunTUIModel(ctx, run),
		tea.WithAltScreen(),
		tea.WithContext(ctx),
	)
	_, err := program.Run()
	return err
}

func newRunTUIModel(ctx context.Context, run assistant.Run) runTUIModel {
	composer := textarea.New()
	composer.Placeholder = "Type here (live submit wiring comes in a later milestone)"
	composer.Prompt = "> "
	composer.ShowLineNumbers = false
	composer.SetHeight(3)
	composer.Focus()

	model := runTUIModel{
		ctx:      ctx,
		run:      run,
		status:   run.Status,
		phase:    run.Phase,
		viewport: viewport.New(0, 0),
		composer: composer,
		activityLines: []string{
			fmt.Sprintf("Run created: %s", run.ID),
			fmt.Sprintf("Chat: %s", run.ChatID),
			"Live activity stream area initialized.",
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
	)
}

func (m runTUIModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tuiContextDoneMsg:
		return m, tea.Quit
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.ready = true
		m.syncLayout()
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "pgup":
			m.viewport.HalfViewUp()
			return m, nil
		case "pgdown":
			m.viewport.HalfViewDown()
			return m, nil
		case "home":
			m.viewport.GotoTop()
			return m, nil
		case "end":
			m.viewport.GotoBottom()
			return m, nil
		}
	}

	var cmds []tea.Cmd
	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	cmds = append(cmds, cmd)
	m.composer, cmd = m.composer.Update(msg)
	cmds = append(cmds, cmd)
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
	request := fmt.Sprintf("Request: %s", truncateForTUI(singleLine(m.run.UserRequestRaw), max(8, m.width-16)))

	body := strings.Join([]string{title, ids, status, request}, "\n")
	return tuiHeaderStyle.
		Width(max(1, m.width)).
		Render(body)
}

func (m runTUIModel) renderActivity() string {
	label := tuiSectionTitleStyle.Render("Activity")
	help := tuiMutedStyle.Render("Scroll: PgUp/PgDn/Home/End")
	body := strings.Join([]string{label, help, m.viewport.View()}, "\n")
	return tuiActivityStyle.
		Width(max(1, m.width)).
		Height(max(3, m.height-lipgloss.Height(m.renderHeader())-lipgloss.Height(m.renderComposer()))).
		Render(body)
}

func (m runTUIModel) renderComposer() string {
	label := tuiSectionTitleStyle.Render("Composer")
	help := tuiMutedStyle.Render("Type your message. Enter currently edits input only. q quits.")
	body := strings.Join([]string{label, m.composer.View(), help}, "\n")
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
