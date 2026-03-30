package ralphloop

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type pageEnvelope struct {
	Command    string           `json:"command"`
	Status     string           `json:"status"`
	Page       int              `json:"page"`
	PageSize   int              `json:"page_size"`
	PageAll    bool             `json:"page_all,omitempty"`
	TotalItems int              `json:"total_items"`
	TotalPages int              `json:"total_pages"`
	Items      []map[string]any `json:"items"`
}

func applyFieldMask(items []map[string]any, fields []string) []map[string]any {
	if len(fields) == 0 {
		return items
	}
	filtered := make([]map[string]any, 0, len(items))
	for _, item := range items {
		record := map[string]any{}
		for _, field := range fields {
			if value, ok := item[field]; ok {
				record[field] = value
			}
		}
		filtered = append(filtered, record)
	}
	return filtered
}

func paginateItems(command string, items []map[string]any, common CommonOptions) []pageEnvelope {
	pageSize := common.PageSize
	if pageSize <= 0 {
		pageSize = 50
	}
	totalItems := len(items)
	totalPages := 1
	if totalItems > 0 {
		totalPages = (totalItems + pageSize - 1) / pageSize
	}
	makeEnvelope := func(page int, chunk []map[string]any) pageEnvelope {
		return pageEnvelope{
			Command:    command,
			Status:     "ok",
			Page:       page,
			PageSize:   pageSize,
			PageAll:    common.PageAll,
			TotalItems: totalItems,
			TotalPages: totalPages,
			Items:      chunk,
		}
	}
	if common.PageAll {
		pages := make([]pageEnvelope, 0, totalPages)
		if totalItems == 0 {
			return []pageEnvelope{makeEnvelope(1, []map[string]any{})}
		}
		for page := 1; page <= totalPages; page++ {
			start := (page - 1) * pageSize
			end := start + pageSize
			if end > totalItems {
				end = totalItems
			}
			pages = append(pages, makeEnvelope(page, items[start:end]))
		}
		return pages
	}
	page := common.Page
	if page <= 0 {
		page = 1
	}
	if totalItems == 0 {
		return []pageEnvelope{makeEnvelope(page, []map[string]any{})}
	}
	if page > totalPages {
		page = totalPages
	}
	start := (page - 1) * pageSize
	end := start + pageSize
	if end > totalItems {
		end = totalItems
	}
	return []pageEnvelope{makeEnvelope(page, items[start:end])}
}

func writeCommandResult(runCtx runContext, payload any) int {
	if runCtx.command.Common.OutputFile != "" {
		resolved, err := sandboxOutputPath(runCtx.invokeCwd, runCtx.command.Common.OutputFile)
		if err != nil {
			return writeCommandError(runCtx.stdout, runCtx.stderr, runCtx.command.Common.Output, string(runCtx.command.Kind), err)
		}
		if err := os.MkdirAll(filepath.Dir(resolved), 0o755); err != nil {
			return writeCommandError(runCtx.stdout, runCtx.stderr, runCtx.command.Common.Output, string(runCtx.command.Kind), err)
		}
		file, err := os.Create(resolved)
		if err != nil {
			return writeCommandError(runCtx.stdout, runCtx.stderr, runCtx.command.Common.Output, string(runCtx.command.Kind), err)
		}
		if err := writeStructuredOutput(file, runCtx.command.Common.Output, payload); err != nil {
			_ = file.Close()
			return writeCommandError(runCtx.stdout, runCtx.stderr, runCtx.command.Common.Output, string(runCtx.command.Kind), err)
		}
		_ = file.Close()
		receipt := map[string]any{
			"command":     runCtx.command.Kind,
			"status":      "ok",
			"output_file": resolved,
		}
		if runCtx.command.Common.Output == OutputText {
			_, _ = fmt.Fprintf(runCtx.stdout, "wrote %s\n", resolved)
			return 0
		}
		if err := writeStructuredOutput(runCtx.stdout, runCtx.command.Common.Output, receipt); err != nil {
			_, _ = fmt.Fprintln(runCtx.stderr, err.Error())
			return 1
		}
		return 0
	}
	if err := writeStructuredOutput(runCtx.stdout, runCtx.command.Common.Output, payload); err != nil {
		_, _ = fmt.Fprintln(runCtx.stderr, err.Error())
		return 1
	}
	return 0
}

func textProgress(runCtx runContext, format string, args ...any) {
	if runCtx.command.Common.Output != OutputText {
		return
	}
	_, _ = fmt.Fprintf(runCtx.textProgress, format, args...)
	if !strings.HasSuffix(format, "\n") {
		_, _ = fmt.Fprintln(runCtx.textProgress)
	}
}
