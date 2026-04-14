package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/siisee11/CodexVirtualAssistant/internal/assistant"
	"github.com/siisee11/CodexVirtualAssistant/internal/store"
)

type watchRunLoader interface {
	ListChats(ctx context.Context) ([]assistant.Chat, error)
	GetRun(ctx context.Context, runID string) (*store.RunRecord, error)
}

var (
	watchListStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("69")).
			Padding(0, 1)
	watchPreviewStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("240")).
				Padding(0, 1)
)

type watchRunItem struct {
	Chat    assistant.Chat `json:"chat"`
	Run     assistant.Run  `json:"run"`
	Preview string         `json:"preview"`
}

type watchPickerModel struct {
	ctx       context.Context
	items     []watchRunItem
	selected  int
	scrollTop int
	width     int
	height    int
	ready     bool
	confirmed bool
}

func loadWatchRunItems(ctx context.Context, client watchRunLoader) ([]watchRunItem, error) {
	if client == nil {
		return nil, fmt.Errorf("missing watch client")
	}
	chats, err := client.ListChats(ctx)
	if err != nil {
		return nil, err
	}

	items := make([]watchRunItem, 0, len(chats))
	for _, chat := range chats {
		record, err := client.GetRun(ctx, chat.LatestRunID)
		if err != nil {
			return nil, err
		}
		if record == nil {
			return nil, fmt.Errorf("missing run record for %s", chat.LatestRunID)
		}
		items = append(items, watchRunItem{
			Chat:    chat,
			Run:     record.Run,
			Preview: buildWatchPreview(record, chat),
		})
	}
	return items, nil
}

func buildWatchPreview(record *store.RunRecord, chat assistant.Chat) string {
	if record == nil {
		return singleLine(chat.Title)
	}
	if record.Run.WaitingFor != nil {
		return singleLine(record.Run.WaitingFor.Prompt)
	}
	if len(record.Events) > 0 {
		summary := strings.TrimSpace(record.Events[len(record.Events)-1].Summary)
		if summary != "" {
			return singleLine(summary)
		}
	}
	if goal := strings.TrimSpace(record.Run.TaskSpec.Goal); goal != "" && goal != strings.TrimSpace(record.Run.UserRequestRaw) {
		return singleLine(goal)
	}
	if request := strings.TrimSpace(record.Run.UserRequestRaw); request != "" {
		return singleLine(request)
	}
	return singleLine(chat.Title)
}

func formatWatchList(items []watchRunItem) string {
	if len(items) == 0 {
		return "No runs found.\n"
	}
	var b strings.Builder
	for _, item := range items {
		fmt.Fprintf(&b, "[%s/%s] %s  %s\n", item.Run.Status, item.Run.Phase, item.Run.ID, truncate(item.Chat.Title, 50))
		fmt.Fprintf(&b, "      Updated: %s\n", item.Chat.UpdatedAt.Local().Format(time.DateTime))
		fmt.Fprintf(&b, "      Request: %s\n", truncate(singleLine(item.Run.UserRequestRaw), 100))
		fmt.Fprintf(&b, "      Preview: %s\n", truncate(item.Preview, 100))
	}
	return b.String()
}

func pickWatchRun(ctx context.Context, items []watchRunItem) (*watchRunItem, error) {
	program := tea.NewProgram(
		newWatchPickerModel(ctx, items),
		tea.WithAltScreen(),
		tea.WithContext(ctx),
	)
	model, err := program.Run()
	if err != nil {
		return nil, err
	}
	finalModel, ok := model.(watchPickerModel)
	if !ok || !finalModel.confirmed || len(finalModel.items) == 0 {
		return nil, nil
	}
	selected := finalModel.items[finalModel.selected]
	return &selected, nil
}

func newWatchPickerModel(ctx context.Context, items []watchRunItem) watchPickerModel {
	return watchPickerModel{ctx: ctx, items: items}
}

func (m watchPickerModel) Init() tea.Cmd {
	return waitForTUIContextDone(m.ctx)
}

func (m watchPickerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
			if len(m.items) == 0 {
				return m, nil
			}
			if m.selected > 0 {
				m.selected--
				m.ensureSelectionVisible()
			}
			return m, nil
		case "down", "j":
			if len(m.items) == 0 {
				return m, nil
			}
			if m.selected < len(m.items)-1 {
				m.selected++
				m.ensureSelectionVisible()
			}
			return m, nil
		case "pgup":
			if len(m.items) == 0 {
				return m, nil
			}
			m.selected = max(0, m.selected-m.visibleListRows())
			m.ensureSelectionVisible()
			return m, nil
		case "pgdown":
			if len(m.items) == 0 {
				return m, nil
			}
			m.selected = min(len(m.items)-1, m.selected+m.visibleListRows())
			m.ensureSelectionVisible()
			return m, nil
		case "home":
			if len(m.items) == 0 {
				return m, nil
			}
			m.selected = 0
			m.ensureSelectionVisible()
			return m, nil
		case "end":
			if len(m.items) == 0 {
				return m, nil
			}
			m.selected = len(m.items) - 1
			m.ensureSelectionVisible()
			return m, nil
		case "enter":
			if len(m.items) == 0 {
				return m, nil
			}
			m.confirmed = true
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m watchPickerModel) View() string {
	if !m.ready {
		return "Loading run list..."
	}

	header := tuiHeaderStyle.Render(lipgloss.NewStyle().
		Width(m.sectionInnerWidth(tuiHeaderStyle)).
		Render(strings.Join([]string{
			tuiSectionTitleStyle.Render("cva watch"),
			truncateForTUI("Select a run and press Enter to reopen the full-screen TUI. q or Esc closes the picker.", m.sectionInnerWidth(tuiHeaderStyle)),
		}, "\n")))

	listPane := watchListStyle.Render(m.renderList())
	previewPane := watchPreviewStyle.Render(m.renderPreview())
	return lipgloss.JoinVertical(lipgloss.Left, header, listPane, previewPane)
}

func (m watchPickerModel) renderList() string {
	if len(m.items) == 0 {
		return "No runs found.\n\nStart one with `cva run \"...\"` and reopen `cva watch`."
	}

	width := max(1, m.sectionInnerWidth(watchListStyle))
	rows := m.visibleListRows()
	start := min(m.scrollTop, max(0, len(m.items)-rows))
	end := min(len(m.items), start+rows)

	lines := make([]string, 0, end-start+1)
	lines = append(lines, tuiSectionTitleStyle.Render("Runs"))
	for idx := start; idx < end; idx++ {
		item := m.items[idx]
		cursor := "  "
		if idx == m.selected {
			cursor = "> "
		}
		label := fmt.Sprintf("%s[%s/%s] %s  %s", cursor, item.Run.Status, item.Run.Phase, item.Run.ID, item.Chat.Title)
		lines = append(lines, truncateForTUI(singleLine(label), width))
	}
	return lipgloss.NewStyle().Width(width).Render(strings.Join(lines, "\n"))
}

func (m watchPickerModel) renderPreview() string {
	width := max(1, m.sectionInnerWidth(watchPreviewStyle))
	if len(m.items) == 0 {
		return lipgloss.NewStyle().Width(width).Render(strings.Join([]string{
			tuiSectionTitleStyle.Render("Preview"),
			"Nothing to preview.",
		}, "\n"))
	}

	item := m.items[m.selected]
	lines := []string{
		tuiSectionTitleStyle.Render("Preview"),
		fmt.Sprintf("Chat: %s", firstNonEmptyTUI(item.Chat.Title, item.Chat.ID)),
		fmt.Sprintf("Run: %s", item.Run.ID),
		fmt.Sprintf("Status: %s   Phase: %s   Updated: %s", item.Run.Status, item.Run.Phase, item.Chat.UpdatedAt.Local().Format(time.DateTime)),
		fmt.Sprintf("Request: %s", singleLine(item.Run.UserRequestRaw)),
		fmt.Sprintf("Latest: %s", firstNonEmptyTUI(item.Preview, "No preview available.")),
	}
	return lipgloss.NewStyle().Width(width).Render(strings.Join(lines, "\n"))
}

func (m *watchPickerModel) clampSelection() {
	if len(m.items) == 0 {
		m.selected = 0
		m.scrollTop = 0
		return
	}
	m.selected = max(0, min(m.selected, len(m.items)-1))
	m.ensureSelectionVisible()
}

func (m *watchPickerModel) ensureSelectionVisible() {
	rows := max(1, m.visibleListRows())
	if m.selected < m.scrollTop {
		m.scrollTop = m.selected
		return
	}
	if m.selected >= m.scrollTop+rows {
		m.scrollTop = m.selected - rows + 1
	}
}

func (m watchPickerModel) visibleListRows() int {
	const reservedHeight = 13
	return max(3, m.height-reservedHeight)
}

func (m watchPickerModel) sectionInnerWidth(style lipgloss.Style) int {
	return max(1, m.width-style.GetHorizontalFrameSize())
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
