package ralphloop

import "fmt"

type schemaField struct {
	Name        string        `json:"name"`
	Description string        `json:"description"`
	Type        string        `json:"type"`
	Required    bool          `json:"required,omitempty"`
	Default     any           `json:"default,omitempty"`
	Enum        []string      `json:"enum,omitempty"`
	Alias       []string      `json:"alias,omitempty"`
	Schema      []schemaField `json:"schema,omitempty"`
}

type commandSchema struct {
	Command        string        `json:"command"`
	Description    string        `json:"description"`
	MutatesState   bool          `json:"mutates_state"`
	SupportsDryRun bool          `json:"supports_dry_run"`
	Positionals    []schemaField `json:"positionals"`
	Options        []schemaField `json:"options"`
	RawPayload     []schemaField `json:"raw_payload_schema"`
}

func commandSchemas() []commandSchema {
	common := []schemaField{
		{Name: "json", Description: "Raw JSON payload or - for stdin", Type: "string"},
		{Name: "output", Description: "Output format", Type: "string", Default: "text|json", Enum: []string{"text", "json", "ndjson"}},
		{Name: "output_file", Description: "Write machine-readable output to a file under the current working directory", Type: "string"},
		{Name: "fields", Description: "Comma-separated field mask for read commands", Type: "string"},
		{Name: "page", Description: "Page number for read commands", Type: "integer", Default: 1},
		{Name: "page_size", Description: "Items per page for read commands", Type: "integer", Default: 50},
		{Name: "page_all", Description: "Read all pages", Type: "boolean", Default: false},
	}
	return []commandSchema{
		{
			Command:        "main",
			Description:    "Run the Ralph loop through setup, coding, and PR phases",
			MutatesState:   true,
			SupportsDryRun: true,
			Positionals: []schemaField{
				{Name: "prompt", Description: "User task prompt", Type: "string", Required: true},
			},
			Options: append(common,
				schemaField{Name: "model", Description: "Codex model", Type: "string", Default: "gpt-5.3-codex"},
				schemaField{Name: "base_branch", Description: "Base branch", Type: "string", Default: "main"},
				schemaField{Name: "max_iterations", Description: "Maximum loop iterations", Type: "integer", Default: 20},
				schemaField{Name: "work_branch", Description: "Working branch name", Type: "string"},
				schemaField{Name: "timeout", Description: "Maximum wall clock time in seconds", Type: "integer", Default: 43200},
				schemaField{Name: "turn_idle_timeout", Description: "Maximum seconds a Codex turn may go without app-server activity before interruption", Type: "integer", Default: 600},
				schemaField{Name: "approval_policy", Description: "Codex approval policy", Type: "string", Default: "never"},
				schemaField{Name: "sandbox", Description: "Codex sandbox policy", Type: "string", Default: "workspace-write"},
				schemaField{Name: "preserve_worktree", Description: "Keep the generated worktree", Type: "boolean", Default: false},
				schemaField{Name: "dry_run", Description: "Validate and describe the request", Type: "boolean", Default: false},
			),
			RawPayload: []schemaField{
				{Name: "command", Description: "Command name", Type: "string", Default: "main"},
				{Name: "prompt", Description: "User task prompt", Type: "string", Required: true},
				{Name: "model", Description: "Codex model", Type: "string", Default: "gpt-5.3-codex"},
				{Name: "base_branch", Description: "Base branch", Type: "string", Default: "main"},
				{Name: "max_iterations", Description: "Maximum loop iterations", Type: "integer", Default: 20},
				{Name: "work_branch", Description: "Working branch name", Type: "string"},
				{Name: "timeout", Description: "Maximum wall clock time in seconds", Type: "integer", Default: 43200},
				{Name: "turn_idle_timeout", Description: "Maximum seconds a Codex turn may go without app-server activity before interruption", Type: "integer", Default: 600},
				{Name: "approval_policy", Description: "Codex approval policy", Type: "string", Default: "never"},
				{Name: "sandbox", Description: "Codex sandbox policy", Type: "string", Default: "workspace-write"},
				{Name: "preserve_worktree", Description: "Keep the generated worktree", Type: "boolean", Default: false},
				{Name: "dry_run", Description: "Validate and describe the request", Type: "boolean", Default: false},
				{Name: "output", Description: "Output format", Type: "string", Enum: []string{"text", "json", "ndjson"}},
			},
		},
		{
			Command:        "init",
			Description:    "Prepare a clean worktree, install dependencies, and verify the build",
			MutatesState:   true,
			SupportsDryRun: true,
			Options: append(common,
				schemaField{Name: "base_branch", Description: "Base branch", Type: "string", Default: "main"},
				schemaField{Name: "work_branch", Description: "Working branch name", Type: "string"},
				schemaField{Name: "dry_run", Description: "Validate and describe the request", Type: "boolean", Default: false},
			),
			RawPayload: []schemaField{
				{Name: "command", Description: "Command name", Type: "string", Default: "init"},
				{Name: "base_branch", Description: "Base branch", Type: "string", Default: "main"},
				{Name: "work_branch", Description: "Working branch name", Type: "string"},
				{Name: "dry_run", Description: "Validate and describe the request", Type: "boolean", Default: false},
				{Name: "output", Description: "Output format", Type: "string", Enum: []string{"text", "json", "ndjson"}},
			},
		},
		{
			Command:        "ls",
			Description:    "List active Ralph loop sessions",
			MutatesState:   false,
			SupportsDryRun: false,
			Positionals: []schemaField{
				{Name: "selector", Description: "Optional session selector", Type: "string"},
			},
			Options:    common,
			RawPayload: []schemaField{{Name: "command", Description: "Command name", Type: "string", Default: "ls"}, {Name: "selector", Description: "Optional selector", Type: "string"}},
		},
		{
			Command:        "tail",
			Description:    "Inspect Ralph loop logs",
			MutatesState:   false,
			SupportsDryRun: false,
			Positionals: []schemaField{
				{Name: "selector", Description: "Optional log selector", Type: "string"},
			},
			Options: append(common,
				schemaField{Name: "lines", Description: "Number of log lines", Type: "integer", Default: 40},
				schemaField{Name: "follow", Description: "Follow appended lines", Type: "boolean", Default: false},
				schemaField{Name: "raw", Description: "Return raw log payloads", Type: "boolean", Default: false},
			),
			RawPayload: []schemaField{
				{Name: "command", Description: "Command name", Type: "string", Default: "tail"},
				{Name: "selector", Description: "Optional selector", Type: "string"},
				{Name: "lines", Description: "Number of log lines", Type: "integer", Default: 40},
				{Name: "follow", Description: "Follow appended lines", Type: "boolean", Default: false},
				{Name: "raw", Description: "Return raw log payloads", Type: "boolean", Default: false},
			},
		},
		{
			Command:        "schema",
			Description:    "Describe the live command schemas",
			MutatesState:   false,
			SupportsDryRun: false,
			Positionals: []schemaField{
				{Name: "command", Description: "Optional command name", Type: "string"},
			},
			Options: append(common,
				schemaField{Name: "command", Description: "Command name to describe", Type: "string"},
			),
			RawPayload: []schemaField{{Name: "command", Description: "Command name", Type: "string", Default: "schema"}},
		},
	}
}

func executeSchemaCommand(runCtx runContext) int {
	items := make([]map[string]any, 0, len(commandSchemas()))
	for _, schema := range commandSchemas() {
		if runCtx.command.SchemaOptions.Command != "" && runCtx.command.SchemaOptions.Command != schema.Command {
			continue
		}
		items = append(items, schemaToMap(schema))
	}
	if runCtx.command.SchemaOptions.Command != "" && len(items) == 0 {
		return writeCommandError(runCtx.stdout, runCtx.stderr, runCtx.command.Common.Output, string(runCtx.command.Kind), fmt.Errorf("unknown command schema: %s", runCtx.command.SchemaOptions.Command))
	}
	items = applyFieldMask(items, runCtx.command.Common.Fields)
	pages := paginateItems("schema", items, runCtx.command.Common)
	if runCtx.command.Common.Output == OutputText {
		return renderSchemaText(runCtx, pages)
	}
	if runCtx.command.Common.Output == OutputNDJSON {
		lines := make([]map[string]any, 0, len(pages))
		for _, page := range pages {
			lines = append(lines, envelopeToMap(page))
		}
		return writeCommandResult(runCtx, lines)
	}
	if runCtx.command.Common.PageAll {
		return writeCommandResult(runCtx, map[string]any{
			"command": "schema",
			"status":  "ok",
			"pages":   pages,
		})
	}
	return writeCommandResult(runCtx, envelopeToMap(pages[0]))
}

func schemaToMap(schema commandSchema) map[string]any {
	return map[string]any{
		"command":          schema.Command,
		"description":      schema.Description,
		"mutates_state":    schema.MutatesState,
		"supports_dry_run": schema.SupportsDryRun,
		"positionals":      schema.Positionals,
		"options":          schema.Options,
		"raw_payload":      schema.RawPayload,
	}
}

func envelopeToMap(page pageEnvelope) map[string]any {
	return map[string]any{
		"command":     page.Command,
		"status":      page.Status,
		"page":        page.Page,
		"page_size":   page.PageSize,
		"page_all":    page.PageAll,
		"total_items": page.TotalItems,
		"total_pages": page.TotalPages,
		"items":       page.Items,
	}
}

func renderSchemaText(runCtx runContext, pages []pageEnvelope) int {
	if len(pages) == 0 || len(pages[0].Items) == 0 {
		_, _ = fmt.Fprintln(runCtx.stdout, "No schema entries matched.")
		return 0
	}
	for _, item := range pages[0].Items {
		_, _ = fmt.Fprintf(runCtx.stdout, "%s: %v\n", item["command"], item["description"])
	}
	return 0
}
