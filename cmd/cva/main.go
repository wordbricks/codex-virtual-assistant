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

	"github.com/siisee11/CodexVirtualAssistant/internal/assistant"
	"github.com/siisee11/CodexVirtualAssistant/internal/app"
	"github.com/siisee11/CodexVirtualAssistant/internal/config"
)

const defaultAddr = "http://127.0.0.1:8080"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	args := os.Args[1:]
	addr := defaultAddr
	jsonMode := false

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
	case "run":
		client := NewClient(addr)
		err = cmdRun(ctx, client, cmdArgs, jsonMode)
	case "status":
		client := NewClient(addr)
		err = cmdStatus(ctx, client, cmdArgs, jsonMode)
	case "list":
		client := NewClient(addr)
		err = cmdList(ctx, client, jsonMode)
	case "chat":
		client := NewClient(addr)
		err = cmdChat(ctx, client, cmdArgs, jsonMode)
	case "watch":
		client := NewClient(addr)
		err = cmdWatch(ctx, client, cmdArgs, jsonMode)
	case "cancel":
		client := NewClient(addr)
		err = cmdCancel(ctx, client, cmdArgs, jsonMode)
	case "resume":
		client := NewClient(addr)
		err = cmdResume(ctx, client, cmdArgs)
	case "schedule":
		client := NewClient(addr)
		err = cmdSchedule(ctx, client, cmdArgs, jsonMode)
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

func cmdStart(ctx context.Context, args []string) error {
	yolo := false
	for _, arg := range args {
		switch arg {
		case "--yolo":
			yolo = true
		default:
			return fmt.Errorf("usage: cva start [--yolo]")
		}
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	if yolo {
		cfg.CodexSandboxMode = "danger-full-access"
		log.Printf("cva start with --yolo; Codex sandbox forced to %s", cfg.CodexSandboxMode)
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

func cmdRun(ctx context.Context, c *Client, args []string, jsonMode bool) error {
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

	if jsonMode {
		return printJSON(resp)
	}

	fmt.Println(formatRunSummary(resp.Run))
	fmt.Printf("Streaming events for %s ...\n\n", resp.Run.ID)

	stream, err := c.StreamEvents(ctx, resp.Run.ID)
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

func cmdStatus(ctx context.Context, c *Client, args []string, jsonMode bool) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: cva status <run_id>")
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

func cmdList(ctx context.Context, c *Client, jsonMode bool) error {
	chats, err := c.ListChats(ctx)
	if err != nil {
		return err
	}
	if jsonMode {
		return printJSON(chats)
	}
	fmt.Print(formatChatList(chats))
	return nil
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

func cmdWatch(ctx context.Context, c *Client, args []string, jsonMode bool) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: cva watch <run_id>")
	}
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

func cmdSchedule(ctx context.Context, c *Client, args []string, jsonMode bool) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: cva schedule <create|list|show|cancel> ...")
	}
	switch args[0] {
	case "create":
		var runID string
		var scheduledFor string
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
		if runID == "" || scheduledFor == "" || len(filtered) == 0 {
			return fmt.Errorf("usage: cva schedule create --run <run_id> --at <scheduled_for> [--max-attempts N] \"prompt\"")
		}
		scheduledRun, err := c.CreateScheduledRun(ctx, runID, scheduledFor, strings.Join(filtered, " "), maxAttempts)
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
  cva [--addr URL] [--json] <command> [args...]

Commands:
  start [--yolo]                        Start the local CVA server
  run [--follow-up <run_id>] "request"   Create a new run and stream events
  status <run_id>                        Show run details
  list                                   List all chats
  chat <chat_id>                         Show chat details
  watch <run_id>                         Stream live events for a run
  cancel <run_id>                        Cancel a running task
  resume <run_id> [key=value ...]        Resume a waiting run with input
  schedule create --run ID --at WHEN "prompt"  Create a scheduled run
  schedule list [--chat ID] [--status S]       List scheduled runs
  schedule show <scheduled_run_id>             Show a scheduled run
  schedule cancel <scheduled_run_id>           Cancel a pending scheduled run

Global Options:
  --addr URL    API server address (default: http://127.0.0.1:8080, env: CVA_ADDR)
  --json        Output raw JSON instead of formatted text
`)
}
