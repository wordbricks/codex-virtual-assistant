package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/siisee11/CodexVirtualAssistant/internal/app"
	"github.com/siisee11/CodexVirtualAssistant/internal/assistant"
	"github.com/siisee11/CodexVirtualAssistant/internal/config"
)

const defaultAddr = "http://127.0.0.1:4999"

type runOutputMode string

const (
	runOutputModeJSON  runOutputMode = "json"
	runOutputModeTUI   runOutputMode = "tui"
	runOutputModePlain runOutputMode = "plain"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	args := os.Args[1:]
	addr := defaultAddr
	jsonMode := false
	interactiveMode := false

	// parse global flags
	var filtered []string
	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "--addr" && i+1 < len(args):
			addr = args[i+1]
			i++
		case strings.HasPrefix(args[i], "--addr="):
			addr = strings.TrimPrefix(args[i], "--addr=")
		case args[i] == "--json":
			jsonMode = true
		case args[i] == "--interactive", args[i] == "-i":
			interactiveMode = true
		default:
			filtered = append(filtered, args[i])
		}
	}
	if env := os.Getenv("CVA_ADDR"); env != "" && addr == defaultAddr {
		addr = env
	}

	if len(filtered) == 0 {
		printUsage()
		os.Exit(1)
	}

	cmd := filtered[0]
	cmdArgs := filtered[1:]

	var err error
	switch cmd {
	case "start":
		err = cmdStart(ctx, cmdArgs)
	case "version", "--version", "-v":
		err = cmdVersion(jsonMode)
	case "upgrade":
		err = cmdUpgrade(ctx, cmdArgs, jsonMode)
	case "logs":
		err = cmdLogs(ctx, cmdArgs)
	case "stop":
		err = cmdStop(cmdArgs)
	case "run":
		client := NewClient(addr)
		err = cmdRun(ctx, client, cmdArgs, jsonMode, interactiveMode)
	case "status":
		client := NewClient(addr)
		err = cmdStatus(ctx, client, cmdArgs, jsonMode)
	case "runtime":
		err = cmdRuntime(cmdArgs, jsonMode)
	case "list":
		client := NewClient(addr)
		err = cmdList(ctx, client, jsonMode, interactiveMode)
	case "chat":
		client := NewClient(addr)
		err = cmdChat(ctx, client, cmdArgs, jsonMode)
	case "watch":
		client := NewClient(addr)
		err = cmdWatch(ctx, client, cmdArgs, jsonMode, interactiveMode)
	case "cancel":
		client := NewClient(addr)
		err = cmdCancel(ctx, client, cmdArgs, jsonMode)
	case "resume":
		client := NewClient(addr)
		err = cmdResume(ctx, client, cmdArgs)
	case "schedule":
		client := NewClient(addr)
		err = cmdSchedule(ctx, client, cmdArgs, jsonMode, interactiveMode)
	case "workspace":
		err = cmdWorkspace(cmdArgs, jsonMode)
	case "help", "--help", "-h":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", cmd)
		printUsage()
		os.Exit(1)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func cmdVersion(jsonMode bool) error {
	info := currentVersionInfo()
	if jsonMode {
		return printJSON(info)
	}
	fmt.Print(formatVersionText(info))
	return nil
}

func cmdStart(ctx context.Context, args []string) error {
	opts, err := parseStartArgs(args)
	if err != nil {
		return err
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	logFile, pidFile, err := resolveDaemonFiles(cfg, opts.logFile, opts.pidFile)
	if err != nil {
		return err
	}

	if opts.yolo {
		cfg.CodexSandboxMode = "danger-full-access"
		log.Printf("cva start with --yolo; Codex sandbox forced to %s", cfg.CodexSandboxMode)
	}
	if opts.daemon && !isDaemonChild() {
		return startDaemonProcess(logFile, pidFile, cfg.HTTPAddr)
	}
	if isDaemonChild() {
		if err := writePIDFile(pidFile, os.Getpid()); err != nil {
			return fmt.Errorf("write pid file: %w", err)
		}
		defer removePIDFileIfOwned(pidFile, os.Getpid())
	}

	application, err := app.New(cfg)
	if err != nil {
		return fmt.Errorf("bootstrap app: %w", err)
	}

	log.Printf("cva server listening on %s", cfg.HTTPAddr)
	if err := application.Run(ctx); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("run app: %w", err)
	}
	return nil
}

func cmdRun(ctx context.Context, c *Client, args []string, jsonMode, interactiveMode bool) error {
	var parentRunID string
	var filtered []string
	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "--follow-up" && i+1 < len(args):
			parentRunID = args[i+1]
			i++
		case strings.HasPrefix(args[i], "--follow-up="):
			parentRunID = strings.TrimPrefix(args[i], "--follow-up=")
		default:
			filtered = append(filtered, args[i])
		}
	}

	if len(filtered) == 0 {
		return fmt.Errorf("usage: cva run [--follow-up <run_id>] \"request\"")
	}
	request := strings.Join(filtered, " ")

	resp, err := c.CreateRun(ctx, request, 0, parentRunID)
	if err != nil {
		return err
	}

	mode := selectRunOutputMode(jsonMode, interactiveMode, isTTY(os.Stdin), isTTY(os.Stdout))
	switch mode {
	case runOutputModeJSON:
		return printJSON(resp)
	case runOutputModeTUI:
		return streamRunTUI(ctx, c, resp.Run)
	default:
		return streamRunPlain(ctx, c, resp.Run)
	}
}

func streamRunPlain(ctx context.Context, c *Client, run assistant.Run) error {
	fmt.Println(formatRunSummary(run))
	fmt.Printf("Streaming events for %s ...\n\n", run.ID)

	stream, err := c.StreamEvents(ctx, run.ID)
	if err != nil {
		return err
	}
	defer stream.Close()

	return streamSSE(stream, func(ev assistant.RunEvent) bool {
		fmt.Println(formatEvent(ev))
		return !isTerminalPhase(ev.Phase)
	})
}

func streamRunTUI(ctx context.Context, c *Client, run assistant.Run) error {
	return runRunTUI(ctx, c, run)
}

func cmdStatus(ctx context.Context, c *Client, args []string, jsonMode bool) error {
	if len(args) == 0 {
		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("load config: %w", err)
		}
		status := localStatusFromConfig(cfg)
		if jsonMode {
			return printJSON(status)
		}
		fmt.Print(formatLocalStatus(status))
		return nil
	}
	if len(args) > 1 {
		return fmt.Errorf("usage: cva status [run_id]")
	}
	rec, err := c.GetRun(ctx, args[0])
	if err != nil {
		return err
	}
	if jsonMode {
		return printJSON(rec)
	}
	fmt.Print(formatRunRecord(rec))
	return nil
}

func cmdList(ctx context.Context, c *Client, jsonMode, interactiveMode bool) error {
	chats, err := c.ListChats(ctx)
	if err != nil {
		return err
	}
	switch selectBrowseOutputMode(jsonMode, interactiveMode, isTTY(os.Stdin), isTTY(os.Stdout)) {
	case browseOutputModeJSON:
		return printJSON(chats)
	case browseOutputModeTUI:
		selected, err := pickChat(ctx, chats)
		if err != nil || selected == nil {
			return err
		}
		rec, err := c.GetChat(ctx, selected.ID)
		if err != nil {
			return err
		}
		return viewChatTUI(ctx, rec)
	default:
		fmt.Print(formatChatList(chats))
		return nil
	}
}

func cmdChat(ctx context.Context, c *Client, args []string, jsonMode bool) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: cva chat <chat_id>")
	}
	rec, err := c.GetChat(ctx, args[0])
	if err != nil {
		return err
	}
	if jsonMode {
		return printJSON(rec)
	}
	fmt.Print(formatChatRecord(rec))
	return nil
}

func cmdWatch(ctx context.Context, c *Client, args []string, jsonMode, interactiveMode bool) error {
	if len(args) > 1 {
		return fmt.Errorf("usage: cva watch [<run_id>]")
	}

	if len(args) == 0 {
		items, err := loadWatchRunItems(ctx, c)
		if err != nil {
			return err
		}
		switch selectBrowseOutputMode(jsonMode, interactiveMode, isTTY(os.Stdin), isTTY(os.Stdout)) {
		case browseOutputModeJSON:
			return printJSON(items)
		case browseOutputModeTUI:
			selected, err := pickWatchRun(ctx, items)
			if err != nil || selected == nil {
				return err
			}
			record, err := c.GetRun(ctx, selected.Run.ID)
			if err != nil {
				return err
			}
			return streamRunTUI(ctx, c, record.Run)
		default:
			fmt.Print(formatWatchList(items))
			return nil
		}
	}

	switch selectBrowseOutputMode(jsonMode, interactiveMode, isTTY(os.Stdin), isTTY(os.Stdout)) {
	case browseOutputModeTUI:
		record, err := c.GetRun(ctx, args[0])
		if err != nil {
			return err
		}
		return streamRunTUI(ctx, c, record.Run)
	default:
		stream, err := c.StreamEvents(ctx, args[0])
		if err != nil {
			return err
		}
		defer stream.Close()

		return streamSSE(stream, func(ev assistant.RunEvent) bool {
			if jsonMode {
				printJSON(ev)
			} else {
				fmt.Println(formatEvent(ev))
			}
			return !isTerminalPhase(ev.Phase)
		})
	}
}

func cmdCancel(ctx context.Context, c *Client, args []string, jsonMode bool) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: cva cancel <run_id>")
	}
	rec, err := c.CancelRun(ctx, args[0])
	if err != nil {
		return err
	}
	if jsonMode {
		return printJSON(rec)
	}
	fmt.Print(formatRunSummary(rec.Run))
	return nil
}

func cmdResume(ctx context.Context, c *Client, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: cva resume <run_id> [key=value ...]")
	}
	runID := args[0]
	input := make(map[string]string)
	for _, kv := range args[1:] {
		parts := strings.SplitN(kv, "=", 2)
		if len(parts) != 2 {
			return fmt.Errorf("invalid input format %q, expected key=value", kv)
		}
		input[parts[0]] = parts[1]
	}
	if err := c.ResumeRun(ctx, runID, input); err != nil {
		return err
	}
	fmt.Println("Run resumed.")
	return nil
}

func cmdSchedule(ctx context.Context, c *Client, args []string, jsonMode, interactiveMode bool) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: cva schedule <create|update|list|show|cancel> ...")
	}
	switch args[0] {
	case "create":
		var runID string
		var scheduledFor string
		var cronExpr string
		maxAttempts := 0
		var filtered []string
		for i := 1; i < len(args); i++ {
			switch {
			case args[i] == "--run" && i+1 < len(args):
				runID = args[i+1]
				i++
			case strings.HasPrefix(args[i], "--run="):
				runID = strings.TrimPrefix(args[i], "--run=")
			case args[i] == "--at" && i+1 < len(args):
				scheduledFor = args[i+1]
				i++
			case strings.HasPrefix(args[i], "--at="):
				scheduledFor = strings.TrimPrefix(args[i], "--at=")
			case args[i] == "--cron" && i+1 < len(args):
				cronExpr = args[i+1]
				i++
			case strings.HasPrefix(args[i], "--cron="):
				cronExpr = strings.TrimPrefix(args[i], "--cron=")
			case args[i] == "--max-attempts" && i+1 < len(args):
				var err error
				maxAttempts, err = parsePositiveInt(args[i+1])
				if err != nil {
					return fmt.Errorf("invalid --max-attempts: %w", err)
				}
				i++
			case strings.HasPrefix(args[i], "--max-attempts="):
				var err error
				maxAttempts, err = parsePositiveInt(strings.TrimPrefix(args[i], "--max-attempts="))
				if err != nil {
					return fmt.Errorf("invalid --max-attempts: %w", err)
				}
			default:
				filtered = append(filtered, args[i])
			}
		}
		if runID == "" || (scheduledFor == "" && cronExpr == "") || len(filtered) == 0 {
			return fmt.Errorf("usage: cva schedule create --run <run_id> (--at <scheduled_for> | --cron <expr>) [--max-attempts N] \"prompt\"")
		}
		scheduledRun, err := c.CreateScheduledRun(ctx, runID, scheduledFor, cronExpr, strings.Join(filtered, " "), maxAttempts)
		if err != nil {
			return err
		}
		if jsonMode {
			return printJSON(scheduledRun)
		}
		fmt.Print(formatScheduledRun(*scheduledRun))
		return nil
	case "update":
		if len(args) < 2 {
			return fmt.Errorf("usage: cva schedule update <id> [--at <scheduled_for>] [--prompt <text>] [--max-attempts N]")
		}
		scheduledRunID := args[1]
		var scheduledFor string
		var cronExpr string
		var prompt string
		maxAttempts := 0
		for i := 2; i < len(args); i++ {
			switch {
			case args[i] == "--at" && i+1 < len(args):
				scheduledFor = args[i+1]
				i++
			case strings.HasPrefix(args[i], "--at="):
				scheduledFor = strings.TrimPrefix(args[i], "--at=")
			case args[i] == "--cron" && i+1 < len(args):
				cronExpr = args[i+1]
				i++
			case strings.HasPrefix(args[i], "--cron="):
				cronExpr = strings.TrimPrefix(args[i], "--cron=")
			case args[i] == "--prompt" && i+1 < len(args):
				prompt = args[i+1]
				i++
			case strings.HasPrefix(args[i], "--prompt="):
				prompt = strings.TrimPrefix(args[i], "--prompt=")
			case args[i] == "--max-attempts" && i+1 < len(args):
				var err error
				maxAttempts, err = parsePositiveInt(args[i+1])
				if err != nil {
					return fmt.Errorf("invalid --max-attempts: %w", err)
				}
				i++
			case strings.HasPrefix(args[i], "--max-attempts="):
				var err error
				maxAttempts, err = parsePositiveInt(strings.TrimPrefix(args[i], "--max-attempts="))
				if err != nil {
					return fmt.Errorf("invalid --max-attempts: %w", err)
				}
			default:
				return fmt.Errorf("unknown schedule update arg: %s", args[i])
			}
		}
		if scheduledFor == "" && cronExpr == "" && prompt == "" && maxAttempts == 0 {
			return fmt.Errorf("usage: cva schedule update <id> [--at <scheduled_for>] [--cron <expr>] [--prompt <text>] [--max-attempts N]")
		}
		scheduledRun, err := c.UpdateScheduledRun(ctx, scheduledRunID, scheduledFor, cronExpr, prompt, maxAttempts)
		if err != nil {
			return err
		}
		if jsonMode {
			return printJSON(scheduledRun)
		}
		fmt.Print(formatScheduledRun(*scheduledRun))
		return nil
	case "list":
		var chatID string
		var status assistant.ScheduledRunStatus
		for i := 1; i < len(args); i++ {
			switch {
			case args[i] == "--chat" && i+1 < len(args):
				chatID = args[i+1]
				i++
			case strings.HasPrefix(args[i], "--chat="):
				chatID = strings.TrimPrefix(args[i], "--chat=")
			case args[i] == "--status" && i+1 < len(args):
				status = assistant.ScheduledRunStatus(args[i+1])
				i++
			case strings.HasPrefix(args[i], "--status="):
				status = assistant.ScheduledRunStatus(strings.TrimPrefix(args[i], "--status="))
			default:
				return fmt.Errorf("unknown schedule list arg: %s", args[i])
			}
		}
		scheduledRuns, err := c.ListScheduledRuns(ctx, chatID, status)
		if err != nil {
			return err
		}
		if jsonMode {
			return printJSON(scheduledRuns)
		}
		if interactiveMode && isTTY(os.Stdin) && isTTY(os.Stdout) {
			selected, err := pickScheduledRun(ctx, scheduledRuns)
			if err != nil || selected == nil {
				return err
			}
			fmt.Print(formatScheduledRun(*selected))
			return nil
		}
		fmt.Print(formatScheduledRunList(scheduledRuns))
		return nil
	case "show":
		if len(args) < 2 {
			return fmt.Errorf("usage: cva schedule show <id>")
		}
		scheduledRun, err := c.GetScheduledRun(ctx, args[1])
		if err != nil {
			return err
		}
		if jsonMode {
			return printJSON(scheduledRun)
		}
		fmt.Print(formatScheduledRun(*scheduledRun))
		return nil
	case "cancel":
		if len(args) < 2 {
			return fmt.Errorf("usage: cva schedule cancel <id>")
		}
		scheduledRun, err := c.CancelScheduledRun(ctx, args[1])
		if err != nil {
			return err
		}
		if jsonMode {
			return printJSON(scheduledRun)
		}
		fmt.Print(formatScheduledRun(*scheduledRun))
		return nil
	default:
		return fmt.Errorf("unknown schedule subcommand: %s", args[0])
	}
}

func isTerminalPhase(p assistant.RunPhase) bool {
	switch p {
	case assistant.RunPhaseCompleted, assistant.RunPhaseFailed, assistant.RunPhaseCancelled:
		return true
	}
	return false
}

func selectRunOutputMode(jsonMode, interactive, stdinIsTTY, stdoutIsTTY bool) runOutputMode {
	if jsonMode {
		return runOutputModeJSON
	}
	if interactive && stdinIsTTY && stdoutIsTTY {
		return runOutputModeTUI
	}
	return runOutputModePlain
}

func isTTY(file *os.File) bool {
	if file == nil {
		return false
	}
	stat, err := file.Stat()
	if err != nil {
		return false
	}
	return (stat.Mode() & os.ModeCharDevice) != 0
}

func parsePositiveInt(raw string) (int, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return 0, fmt.Errorf("value is required")
	}
	var parsed int
	if _, err := fmt.Sscanf(value, "%d", &parsed); err != nil {
		return 0, err
	}
	if parsed <= 0 {
		return 0, fmt.Errorf("must be positive")
	}
	return parsed, nil
}

func printJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func printUsage() {
	fmt.Print(`cva - Codex Virtual Assistant CLI

Usage:
  cva [--addr URL] [--json] [--interactive|-i] <command> [args...]

Commands:
  start [--yolo] [--daemon] [--log-file PATH] [--pid-file PATH]
                                         Start the local CVA server
  version                                Print CLI version information
  upgrade                                Download and install the latest CLI release
  logs [--follow] [--lines N] [--log-file PATH]
                                         Show daemon log output
  stop [--pid-file PATH]                 Stop the local CVA daemon
  run [--follow-up <run_id>] "request"   Create a new run and stream events
  status [run_id]                        Show local config paths or run details
  runtime [codex|claude|zai]             Show or change the execution runtime
  list                                   List all chats
  chat <chat_id>                         Show chat details
  watch [<run_id>]                       Stream a run or browse recent runs
  cancel <run_id>                        Cancel a running task
  resume <run_id> [key=value ...]        Resume a waiting run with input
  schedule create --run ID --at WHEN "prompt"                   Create a scheduled run
  schedule update <scheduled_run_id> [--at WHEN] [--prompt P]   Update a pending scheduled run
  schedule list [--chat ID] [--status S]                        List scheduled runs
  schedule show <scheduled_run_id>                              Show a scheduled run
  schedule cancel <scheduled_run_id>                            Cancel a pending scheduled run
  workspace lint [project_slug ...]                             Lint project workspace structure

Global Options:
  --addr URL    API server address (default: http://127.0.0.1:4999, env: CVA_ADDR)
  --json        Output raw JSON instead of formatted text
  --interactive, -i
                 Enable TUI for supported commands when stdin/stdout are TTYs
`)
}
