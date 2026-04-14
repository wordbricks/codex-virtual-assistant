package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/siisee11/CodexVirtualAssistant/internal/assistant"
	"github.com/siisee11/CodexVirtualAssistant/internal/store"
)

type chatPickerModel struct {
	ctx       context.Context
	chats     []assistant.Chat
	selected  int
	scrollTop int
	width     int
	height    int
	ready     bool
	confirmed bool
}

type chatViewerModel struct {
	ctx      context.Context
	record   *store.ChatRecord
	width    int
	height   int
	ready    bool
	viewport viewport.Model
}

func pickChat(ctx context.Context, chats []assistant.Chat) (*assistant.Chat, error) {
	program := tea.NewProgram(
		newChatPickerModel(ctx, chats),
		tea.WithAltScreen(),
		tea.WithContext(ctx),
	)
	model, err := program.Run()
	if err != nil {
		return nil, err
	}
	finalModel, ok := model.(chatPickerModel)
	if !ok || !finalModel.confirmed || len(finalModel.chats) == 0 {
		return nil, nil
	}
	selected := finalModel.chats[finalModel.selected]
	return &selected, nil
}

func newChatPickerModel(ctx context.Context, chats []assistant.Chat) chatPickerModel {
	return chatPickerModel{ctx: ctx, chats: chats}
}

func (m chatPickerModel) Init() tea.Cmd {
	return waitForTUIContextDone(m.ctx)
}

func (m chatPickerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tuiContextDoneMsg:
		return m, tea.Quit
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.ready = true
		m.clampSelection()
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q", "esc":
			return m, tea.Quit
		case "up", "k":
			if len(m.chats) == 0 {
				return m, nil
			}
			if m.selected > 0 {
				m.selected--
				m.ensureSelectionVisible()
			}
			return m, nil
		case "down", "j":
			if len(m.chats) == 0 {
				return m, nil
			}
			if m.selected < len(m.chats)-1 {
				m.selected++
				m.ensureSelectionVisible()
			}
			return m, nil
		case "pgup":
			if len(m.chats) == 0 {
				return m, nil
			}
			m.selected = max(0, m.selected-m.visibleListRows())
			m.ensureSelectionVisible()
			return m, nil
		case "pgdown":
			if len(m.chats) == 0 {
				return m, nil
			}
			m.selected = min(len(m.chats)-1, m.selected+m.visibleListRows())
			m.ensureSelectionVisible()
			return m, nil
		case "home":
			if len(m.chats) == 0 {
				return m, nil
			}
			m.selected = 0
			m.ensureSelectionVisible()
			return m, nil
		case "end":
			if len(m.chats) == 0 {
				return m, nil
			}
			m.selected = len(m.chats) - 1
			m.ensureSelectionVisible()
			return m, nil
		case "enter":
			if len(m.chats) == 0 {
				return m, nil
			}
			m.confirmed = true
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m chatPickerModel) View() string {
	if !m.ready {
		return "Loading chat list..."
	}

	header := tuiHeaderStyle.Render(lipgloss.NewStyle().
		Width(m.sectionInnerWidth(tuiHeaderStyle)).
		Render(strings.Join([]string{
			tuiSectionTitleStyle.Render("cva list"),
			truncateForTUI("Select a chat and press Enter to inspect it. q or Esc closes the picker.", m.sectionInnerWidth(tuiHeaderStyle)),
		}, "\n")))

	listPane := watchListStyle.Render(m.renderList())
	previewPane := watchPreviewStyle.Render(m.renderPreview())
	return lipgloss.JoinVertical(lipgloss.Left, header, listPane, previewPane)
}

func (m chatPickerModel) renderList() string {
	if len(m.chats) == 0 {
		return "No chats found.\n\nStart one with `cva run \"...\"` and reopen `cva list`."
	}

	width := max(1, m.sectionInnerWidth(watchListStyle))
	rows := m.visibleListRows()
	start := min(m.scrollTop, max(0, len(m.chats)-rows))
	end := min(len(m.chats), start+rows)

	lines := make([]string, 0, end-start+1)
	lines = append(lines, tuiSectionTitleStyle.Render("Chats"))
	for idx := start; idx < end; idx++ {
		chat := m.chats[idx]
		cursor := "  "
		if idx == m.selected {
			cursor = "> "
		}
		label := fmt.Sprintf("%s[%s] %s  %s", cursor, chat.Status, chat.ID, chat.Title)
		lines = append(lines, truncateForTUI(singleLine(label), width))
	}
	return lipgloss.NewStyle().Width(width).Render(strings.Join(lines, "\n"))
}

func (m chatPickerModel) renderPreview() string {
	width := max(1, m.sectionInnerWidth(watchPreviewStyle))
	if len(m.chats) == 0 {
		return lipgloss.NewStyle().Width(width).Render(strings.Join([]string{
			tuiSectionTitleStyle.Render("Preview"),
			"Nothing to preview.",
		}, "\n"))
	}

	chat := m.chats[m.selected]
	lines := []string{
		tuiSectionTitleStyle.Render("Preview"),
		fmt.Sprintf("Title: %s", firstNonEmptyTUI(chat.Title, chat.ID)),
		fmt.Sprintf("Chat: %s", chat.ID),
		fmt.Sprintf("Status: %s   Updated: %s", chat.Status, chat.UpdatedAt.Local().Format("2006-01-02 15:04:05")),
		fmt.Sprintf("Latest run: %s", firstNonEmptyTUI(chat.LatestRunID, "None")),
		fmt.Sprintf("Root run: %s", firstNonEmptyTUI(chat.RootRunID, "None")),
	}
	return lipgloss.NewStyle().Width(width).Render(strings.Join(lines, "\n"))
}

func (m *chatPickerModel) clampSelection() {
	if len(m.chats) == 0 {
		m.selected = 0
		m.scrollTop = 0
		return
	}
	m.selected = max(0, min(m.selected, len(m.chats)-1))
	m.ensureSelectionVisible()
}

func (m *chatPickerModel) ensureSelectionVisible() {
	rows := max(1, m.visibleListRows())
	if m.selected < m.scrollTop {
		m.scrollTop = m.selected
		return
	}
	if m.selected >= m.scrollTop+rows {
		m.scrollTop = m.selected - rows + 1
	}
}

func (m chatPickerModel) visibleListRows() int {
	const reservedHeight = 13
	return max(3, m.height-reservedHeight)
}

func (m chatPickerModel) sectionInnerWidth(style lipgloss.Style) int {
	return max(1, m.width-style.GetHorizontalFrameSize())
}

func viewChatTUI(ctx context.Context, rec *store.ChatRecord) error {
	program := tea.NewProgram(
		newChatViewerModel(ctx, rec),
		tea.WithAltScreen(),
		tea.WithContext(ctx),
	)
	_, err := program.Run()
	return err
}

func newChatViewerModel(ctx context.Context, rec *store.ChatRecord) chatViewerModel {
	model := chatViewerModel{
		ctx:      ctx,
		record:   rec,
		viewport: viewport.New(0, 0),
	}
	model.refreshViewportContent()
	return model
}

func (m chatViewerModel) Init() tea.Cmd {
	return waitForTUIContextDone(m.ctx)
}

func (m chatViewerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
		case "ctrl+c", "q", "esc":
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

	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

func (m chatViewerModel) View() string {
	if !m.ready {
		return "Loading chat details..."
	}
	return lipgloss.JoinVertical(lipgloss.Left, m.renderHeader(), m.renderBody())
}

func (m *chatViewerModel) syncLayout() {
	if !m.ready {
		return
	}

	headerHeight := lipgloss.Height(m.renderHeader())
	bodyOuterHeight := max(3, m.height-headerHeight)
	bodyInnerHeight := max(1, bodyOuterHeight-tuiActivityStyle.GetVerticalFrameSize())

	m.viewport.Width = m.sectionInnerWidth(tuiActivityStyle)
	metaHeight := lipgloss.Height(m.renderBodyMeta())
	m.viewport.Height = max(1, bodyInnerHeight-metaHeight)
	m.refreshViewportContent()
}

func (m chatViewerModel) renderHeader() string {
	innerWidth := m.sectionInnerWidth(tuiHeaderStyle)
	title := tuiSectionTitleStyle.Render("cva list")
	chatID := fmt.Sprintf("Chat: %s", m.chatID())
	status := fmt.Sprintf("Status: %s   Runs: %d", m.chatStatus(), m.runCount())
	updated := fmt.Sprintf("Updated: %s", m.chatUpdatedAt())
	body := lipgloss.NewStyle().
		Width(innerWidth).
		Render(strings.Join([]string{
			title,
			truncateForTUI(chatID, innerWidth),
			truncateForTUI(status, innerWidth),
			truncateForTUI(updated, innerWidth),
		}, "\n"))
	return tuiHeaderStyle.Render(body)
}

func (m chatViewerModel) renderBodyMeta() string {
	lines := []string{
		tuiSectionTitleStyle.Render("Chat Detail"),
		tuiMutedStyle.Render("Scroll: Arrow keys/j/k/PgUp/PgDn/Home/End   q or Esc closes the viewer"),
	}
	return lipgloss.NewStyle().
		Width(m.sectionInnerWidth(tuiActivityStyle)).
		Render(strings.Join(lines, "\n"))
}

func (m chatViewerModel) renderBody() string {
	body := lipgloss.JoinVertical(lipgloss.Left, m.renderBodyMeta(), m.viewport.View())
	return tuiActivityStyle.
		Height(max(1, m.bodyOuterHeight()-tuiActivityStyle.GetVerticalFrameSize())).
		Render(body)
}

func (m *chatViewerModel) refreshViewportContent() {
	content := "Chat details are unavailable."
	if m.record != nil {
		content = formatChatRecord(m.record)
	}
	if m.viewport.Width > 0 {
		content = lipgloss.NewStyle().
			Width(m.viewport.Width).
			Render(content)
	}
	m.viewport.SetContent(content)
}

func (m chatViewerModel) sectionInnerWidth(style lipgloss.Style) int {
	return max(1, m.width-style.GetHorizontalFrameSize())
}

func (m chatViewerModel) bodyOuterHeight() int {
	return max(3, m.height-lipgloss.Height(m.renderHeader()))
}

func (m chatViewerModel) chatID() string {
	if m.record == nil {
		return "unknown"
	}
	return firstNonEmptyTUI(m.record.Chat.ID, "unknown")
}

func (m chatViewerModel) chatStatus() assistant.RunStatus {
	if m.record == nil {
		return ""
	}
	return m.record.Chat.Status
}

func (m chatViewerModel) chatUpdatedAt() string {
	if m.record == nil || m.record.Chat.UpdatedAt.IsZero() {
		return "unknown"
	}
	return m.record.Chat.UpdatedAt.Local().Format("2006-01-02 15:04:05")
}

func (m chatViewerModel) runCount() int {
	if m.record == nil {
		return 0
	}
	return len(m.record.Runs)
}
