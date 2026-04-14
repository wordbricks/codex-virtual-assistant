package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/siisee11/CodexVirtualAssistant/internal/assistant"
)

var (
	scheduleListStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("69")).
				Padding(0, 1)
	schedulePreviewStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("240")).
				Padding(0, 1)
)

const scheduleListFixedRows = 8

type scheduleFocusPane int

const (
	scheduleFocusList scheduleFocusPane = iota
	scheduleFocusPreview
)

type schedulePickerModel struct {
	ctx         context.Context
	allItems    []assistant.ScheduledRun
	items       []assistant.ScheduledRun
	filterQuery string
	selected    int
	scrollTop   int
	width       int
	height      int
	ready       bool
	confirmed   bool
	focus       scheduleFocusPane
	preview     viewport.Model
}

func pickScheduledRun(ctx context.Context, items []assistant.ScheduledRun) (*assistant.ScheduledRun, error) {
	program := tea.NewProgram(
		newSchedulePickerModel(ctx, items),
		tea.WithAltScreen(),
		tea.WithContext(ctx),
	)
	model, err := program.Run()
	if err != nil {
		return nil, err
	}
	finalModel, ok := model.(schedulePickerModel)
	if !ok || !finalModel.confirmed || len(finalModel.items) == 0 {
		return nil, nil
	}
	selected := finalModel.items[finalModel.selected]
	return &selected, nil
}

func newSchedulePickerModel(ctx context.Context, items []assistant.ScheduledRun) schedulePickerModel {
	return schedulePickerModel{
		ctx:      ctx,
		allItems: items,
		items:    items,
		focus:    scheduleFocusList,
		preview:  viewport.New(0, 0),
	}
}

func (m schedulePickerModel) Init() tea.Cmd {
	return waitForTUIContextDone(m.ctx)
}

func (m schedulePickerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tuiContextDoneMsg:
		return m, tea.Quit
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.ready = true
		m.clampSelection()
		m.syncLayout(false)
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q", "esc":
			return m, tea.Quit
		case "tab":
			if m.focus == scheduleFocusList {
				m.focus = scheduleFocusPreview
			} else {
				m.focus = scheduleFocusList
			}
			return m, nil
		case "backspace":
			if m.focus != scheduleFocusList {
				return m, nil
			}
			if m.filterQuery == "" {
				return m, nil
			}
			m.filterQuery = m.filterQuery[:len(m.filterQuery)-1]
			m.applyFilter()
			return m, nil
		case "ctrl+u":
			if m.focus == scheduleFocusPreview {
				m.preview.GotoTop()
				return m, nil
			}
			if m.filterQuery == "" {
				return m, nil
			}
			m.filterQuery = ""
			m.applyFilter()
			return m, nil
		case "up", "k":
			if m.focus == scheduleFocusPreview {
				m.preview.LineUp(1)
				return m, nil
			}
			if len(m.items) == 0 {
				return m, nil
			}
			if m.selected > 0 {
				m.selected--
				m.ensureSelectionVisible()
				m.syncLayout(true)
			}
			return m, nil
		case "down", "j":
			if m.focus == scheduleFocusPreview {
				m.preview.LineDown(1)
				return m, nil
			}
			if len(m.items) == 0 {
				return m, nil
			}
			if m.selected < len(m.items)-1 {
				m.selected++
				m.ensureSelectionVisible()
				m.syncLayout(true)
			}
			return m, nil
		case "pgup":
			if m.focus == scheduleFocusPreview {
				m.preview.HalfViewUp()
				return m, nil
			}
			if len(m.items) == 0 {
				return m, nil
			}
			m.selected = max(0, m.selected-m.visibleListRows())
			m.ensureSelectionVisible()
			m.syncLayout(true)
			return m, nil
		case "pgdown":
			if m.focus == scheduleFocusPreview {
				m.preview.HalfViewDown()
				return m, nil
			}
			if len(m.items) == 0 {
				return m, nil
			}
			m.selected = min(len(m.items)-1, m.selected+m.visibleListRows())
			m.ensureSelectionVisible()
			m.syncLayout(true)
			return m, nil
		case "home":
			if m.focus == scheduleFocusPreview {
				m.preview.GotoTop()
				return m, nil
			}
			if len(m.items) == 0 {
				return m, nil
			}
			m.selected = 0
			m.ensureSelectionVisible()
			m.syncLayout(true)
			return m, nil
		case "end":
			if m.focus == scheduleFocusPreview {
				m.preview.GotoBottom()
				return m, nil
			}
			if len(m.items) == 0 {
				return m, nil
			}
			m.selected = len(m.items) - 1
			m.ensureSelectionVisible()
			m.syncLayout(true)
			return m, nil
		case "enter":
			if len(m.items) == 0 {
				return m, nil
			}
			m.confirmed = true
			return m, tea.Quit
		default:
			if m.focus == scheduleFocusList && msg.Type == tea.KeyRunes {
				m.filterQuery += msg.String()
				m.applyFilter()
				return m, nil
			}
		}
	}
	return m, nil
}

func (m schedulePickerModel) View() string {
	if !m.ready {
		return "Loading scheduled runs..."
	}

	header := tuiHeaderStyle.Render(lipgloss.NewStyle().
		Width(m.sectionInnerWidth(tuiHeaderStyle)).
		Render(strings.Join([]string{
			tuiSectionTitleStyle.Render("cva schedule list --interactive"),
			truncateForTUI("Type to filter in the list. Tab switches focus. List focus: arrows move selection. Preview focus: arrows scroll. Enter prints the selected schedule. q or Esc closes.", m.sectionInnerWidth(tuiHeaderStyle)),
		}, "\n")))

	listPane := scheduleListStyle.Render(m.renderList())
	previewPane := schedulePreviewStyle.Render(m.renderPreview())
	return lipgloss.JoinVertical(lipgloss.Left, header, listPane, previewPane)
}

func (m schedulePickerModel) renderList() string {
	rows := m.visibleListRows()
	if len(m.items) == 0 {
		lines := []string{
			tuiSectionTitleStyle.Render("Scheduled Runs"),
			truncateForTUI(fmt.Sprintf("Filter: %s", firstNonEmptyTUI(m.filterQuery, "(none)")), max(1, m.sectionInnerWidth(scheduleListStyle))),
			"",
			"No matching scheduled runs found.",
			"",
			"Adjust the filter or create one with `cva schedule create ...`.",
		}
		for len(lines) < rows+1 {
			lines = append(lines, "")
		}
		return strings.Join(lines[:rows+1], "\n")
	}

	width := max(1, m.sectionInnerWidth(scheduleListStyle))
	start := min(m.scrollTop, max(0, len(m.items)-rows))
	end := min(len(m.items), start+rows)

	title := "Scheduled Runs"
	if m.focus == scheduleFocusList {
		title += " [focus]"
	}
	lines := make([]string, 0, end-start+2)
	lines = append(lines, tuiSectionTitleStyle.Render(title))
	lines = append(lines, truncateForTUI(fmt.Sprintf("Filter: %s", firstNonEmptyTUI(m.filterQuery, "(none)")), width))
	for idx := start; idx < end; idx++ {
		item := m.items[idx]
		cursor := "  "
		if idx == m.selected {
			cursor = "> "
		}
		label := fmt.Sprintf("%s[%s] %s  %s", cursor, item.Status, item.ID, item.ScheduledFor.Local().Format(time.DateTime))
		lines = append(lines, truncateForTUI(singleLine(label), width))
	}
	for len(lines) < rows+1 {
		lines = append(lines, "")
	}
	return lipgloss.NewStyle().Width(width).Render(strings.Join(lines, "\n"))
}

func (m schedulePickerModel) renderPreview() string {
	width := max(1, m.sectionInnerWidth(schedulePreviewStyle))
	title := "Preview"
	if m.focus == scheduleFocusPreview {
		title += " [focus]"
	}
	if len(m.items) == 0 {
		lines := []string{
			tuiSectionTitleStyle.Render(title),
			"Nothing to preview.",
		}
		return lipgloss.NewStyle().Width(width).Render(strings.Join(lines, "\n"))
	}

	lines := []string{
		tuiSectionTitleStyle.Render(title),
		truncateForTUI(fmt.Sprintf("Matches: %d / %d", len(m.items), len(m.allItems)), width),
		tuiMutedStyle.Render(truncateForTUI("Scroll: Up/Down/PgUp/PgDn/Home/End when preview is focused", width)),
		"",
	}
	lines = append(lines, strings.Split(m.preview.View(), "\n")...)
	return lipgloss.NewStyle().Width(width).Render(strings.Join(lines, "\n"))
}

func (m *schedulePickerModel) clampSelection() {
	if len(m.items) == 0 {
		m.selected = 0
		m.scrollTop = 0
		m.syncLayout(true)
		return
	}
	m.selected = max(0, min(m.selected, len(m.items)-1))
	m.ensureSelectionVisible()
	m.syncLayout(true)
}

func (m *schedulePickerModel) applyFilter() {
	if strings.TrimSpace(m.filterQuery) == "" {
		m.items = m.allItems
		m.selected = 0
		m.scrollTop = 0
		m.clampSelection()
		return
	}

	query := strings.ToLower(strings.TrimSpace(m.filterQuery))
	filtered := make([]assistant.ScheduledRun, 0, len(m.allItems))
	for _, item := range m.allItems {
		if strings.Contains(strings.ToLower(scheduleSearchBlob(item)), query) {
			filtered = append(filtered, item)
		}
	}
	m.items = filtered
	m.selected = 0
	m.scrollTop = 0
	m.clampSelection()
}

func (m *schedulePickerModel) ensureSelectionVisible() {
	rows := max(1, m.visibleListRows())
	if m.selected < m.scrollTop {
		m.scrollTop = m.selected
		return
	}
	if m.selected >= m.scrollTop+rows {
		m.scrollTop = m.selected - rows + 1
	}
}

func (m schedulePickerModel) visibleListRows() int {
	if m.height <= 0 {
		return scheduleListFixedRows
	}
	maxRows := m.height - m.headerHeight() - schedulePreviewStyle.GetVerticalFrameSize() - 2
	if maxRows <= 0 {
		return 1
	}
	return min(scheduleListFixedRows, maxRows)
}

func (m schedulePickerModel) sectionInnerWidth(style lipgloss.Style) int {
	return max(1, m.width-style.GetHorizontalFrameSize())
}

func (m schedulePickerModel) headerHeight() int {
	const headerContentRows = 2
	return headerContentRows + tuiHeaderStyle.GetVerticalFrameSize()
}

func (m schedulePickerModel) previewHeight() int {
	return m.previewContentRows() + schedulePreviewStyle.GetVerticalFrameSize()
}

func (m schedulePickerModel) previewContentRows() int {
	if m.height <= 0 {
		return 8
	}
	remaining := m.height - m.headerHeight() - (m.visibleListRows() + scheduleListStyle.GetVerticalFrameSize()) - schedulePreviewStyle.GetVerticalFrameSize()
	if remaining <= 0 {
		return 2
	}
	return remaining
}

func (m *schedulePickerModel) syncLayout(resetPreview bool) {
	if !m.ready {
		return
	}
	m.preview.Width = max(1, m.sectionInnerWidth(schedulePreviewStyle))
	m.preview.Height = max(1, m.previewContentRows()-4)
	m.refreshPreviewContent(resetPreview)
}

func (m *schedulePickerModel) refreshPreviewContent(resetScroll bool) {
	if len(m.items) == 0 {
		m.preview.SetContent("Nothing to preview.")
		if resetScroll {
			m.preview.GotoTop()
		}
		return
	}
	m.preview.SetContent(buildScheduledRunPreviewContent(m.items[m.selected], m.preview.Width))
	if resetScroll {
		m.preview.GotoTop()
	}
}

func buildScheduledRunPreviewContent(item assistant.ScheduledRun, width int) string {
	lines := []string{
		truncateForTUI(fmt.Sprintf("ID: %s", item.ID), width),
		truncateForTUI(fmt.Sprintf("Status: %s", item.Status), width),
		truncateForTUI(fmt.Sprintf("Scheduled: %s", item.ScheduledFor.Local().Format(time.DateTime)), width),
		truncateForTUI(fmt.Sprintf("Created: %s", item.CreatedAt.Local().Format(time.DateTime)), width),
		truncateForTUI(fmt.Sprintf("Chat: %s", item.ChatID), width),
		truncateForTUI(fmt.Sprintf("Parent run: %s", item.ParentRunID), width),
		truncateForTUI(fmt.Sprintf("Max attempts: %d", item.MaxGenerationAttempts), width),
	}
	if item.TriggeredAt != nil {
		lines = append(lines, truncateForTUI(fmt.Sprintf("Triggered: %s", item.TriggeredAt.Local().Format(time.DateTime)), width))
	}
	if item.RunID != "" {
		lines = append(lines, truncateForTUI(fmt.Sprintf("Triggered run: %s", item.RunID), width))
	}
	if item.ErrorMessage != "" {
		lines = append(lines,
			"",
			tuiSectionTitleStyle.Render("Error"),
			wrapForTUI(singleLine(item.ErrorMessage), width),
		)
	}
	lines = append(lines,
		"",
		tuiSectionTitleStyle.Render("Prompt"),
		wrapForTUI(singleLine(item.UserRequestRaw), width),
	)
	return strings.Join(lines, "\n")
}

func wrapForTUI(value string, width int) string {
	if width <= 0 {
		return value
	}
	words := strings.Fields(value)
	if len(words) == 0 {
		return ""
	}
	lines := make([]string, 0, len(words))
	current := words[0]
	for _, word := range words[1:] {
		candidate := current + " " + word
		if len(candidate) <= width {
			current = candidate
			continue
		}
		lines = append(lines, current)
		current = word
	}
	lines = append(lines, current)
	return strings.Join(lines, "\n")
}

func scheduleSearchBlob(item assistant.ScheduledRun) string {
	parts := []string{
		item.ID,
		string(item.Status),
		item.ChatID,
		item.ParentRunID,
		item.RunID,
		item.UserRequestRaw,
		item.ErrorMessage,
		item.ScheduledFor.Local().Format(time.DateTime),
		item.CreatedAt.Local().Format(time.DateTime),
	}
	if item.TriggeredAt != nil {
		parts = append(parts, item.TriggeredAt.Local().Format(time.DateTime))
	}
	return strings.Join(parts, "\n")
}
