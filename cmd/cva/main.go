package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/siisee11/CodexVirtualAssistant/internal/assistant"
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

	client := NewClient(addr)
	cmd := filtered[0]
	cmdArgs := filtered[1:]

	var err error
	switch cmd {
	case "run":
		err = cmdRun(ctx, client, cmdArgs, jsonMode)
	case "status":
		err = cmdStatus(ctx, client, cmdArgs, jsonMode)
	case "list":
		err = cmdList(ctx, client, jsonMode)
	case "chat":
		err = cmdChat(ctx, client, cmdArgs, jsonMode)
	case "watch":
		err = cmdWatch(ctx, client, cmdArgs, jsonMode)
	case "cancel":
		err = cmdCancel(ctx, client, cmdArgs, jsonMode)
	case "resume":
		err = cmdResume(ctx, client, cmdArgs)
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

func isTerminalPhase(p assistant.RunPhase) bool {
	switch p {
	case assistant.RunPhaseCompleted, assistant.RunPhaseFailed, assistant.RunPhaseCancelled:
		return true
	}
	return false
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
  run [--follow-up <run_id>] "request"   Create a new run and stream events
  status <run_id>                        Show run details
  list                                   List all chats
  chat <chat_id>                         Show chat details
  watch <run_id>                         Stream live events for a run
  cancel <run_id>                        Cancel a running task
  resume <run_id> [key=value ...]        Resume a waiting run with input

Global Options:
  --addr URL    API server address (default: http://127.0.0.1:8080, env: CVA_ADDR)
  --json        Output raw JSON instead of formatted text
`)
}
