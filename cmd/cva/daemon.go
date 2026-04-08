package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/siisee11/CodexVirtualAssistant/internal/config"
)

const (
	daemonChildEnv   = "CVA_DAEMON_CHILD"
	daemonLogFileEnv = "CVA_DAEMON_LOG_FILE"
	daemonPIDFileEnv = "CVA_DAEMON_PID_FILE"
)

type startOptions struct {
	yolo    bool
	daemon  bool
	logFile string
	pidFile string
}

type logsOptions struct {
	logFile string
	lines   int
	follow  bool
}

func parseStartArgs(args []string) (startOptions, error) {
	opts := startOptions{}
	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "--yolo":
			opts.yolo = true
		case args[i] == "--daemon":
			opts.daemon = true
		case args[i] == "--log-file" && i+1 < len(args):
			opts.logFile = args[i+1]
			i++
		case strings.HasPrefix(args[i], "--log-file="):
			opts.logFile = strings.TrimPrefix(args[i], "--log-file=")
		case args[i] == "--pid-file" && i+1 < len(args):
			opts.pidFile = args[i+1]
			i++
		case strings.HasPrefix(args[i], "--pid-file="):
			opts.pidFile = strings.TrimPrefix(args[i], "--pid-file=")
		default:
			return startOptions{}, fmt.Errorf("usage: cva start [--yolo] [--daemon] [--log-file PATH] [--pid-file PATH]")
		}
	}
	return opts, nil
}

func parseLogsArgs(args []string) (logsOptions, error) {
	opts := logsOptions{lines: 100}
	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "--follow":
			opts.follow = true
		case args[i] == "--lines" && i+1 < len(args):
			value, err := parsePositiveInt(args[i+1])
			if err != nil {
				return logsOptions{}, fmt.Errorf("parse --lines: %w", err)
			}
			opts.lines = value
			i++
		case strings.HasPrefix(args[i], "--lines="):
			value, err := parsePositiveInt(strings.TrimPrefix(args[i], "--lines="))
			if err != nil {
				return logsOptions{}, fmt.Errorf("parse --lines: %w", err)
			}
			opts.lines = value
		case args[i] == "--log-file" && i+1 < len(args):
			opts.logFile = args[i+1]
			i++
		case strings.HasPrefix(args[i], "--log-file="):
			opts.logFile = strings.TrimPrefix(args[i], "--log-file=")
		default:
			return logsOptions{}, fmt.Errorf("usage: cva logs [--follow] [--lines N] [--log-file PATH]")
		}
	}
	return opts, nil
}

func parseStopArgs(args []string) (string, error) {
	var pidFile string
	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "--pid-file" && i+1 < len(args):
			pidFile = args[i+1]
			i++
		case strings.HasPrefix(args[i], "--pid-file="):
			pidFile = strings.TrimPrefix(args[i], "--pid-file=")
		default:
			return "", fmt.Errorf("usage: cva stop [--pid-file PATH]")
		}
	}
	return pidFile, nil
}

func resolveDaemonFiles(cfg config.Config, logFileOverride, pidFileOverride string) (string, string, error) {
	logFile := os.Getenv(daemonLogFileEnv)
	if strings.TrimSpace(logFile) == "" {
		logFile = logFileOverride
	}
	if strings.TrimSpace(logFile) == "" {
		logFile = filepath.Join(cfg.DataDir, "logs", "cva.log")
	}

	pidFile := os.Getenv(daemonPIDFileEnv)
	if strings.TrimSpace(pidFile) == "" {
		pidFile = pidFileOverride
	}
	if strings.TrimSpace(pidFile) == "" {
		pidFile = filepath.Join(cfg.DataDir, "cva.pid")
	}

	absLog, err := filepath.Abs(logFile)
	if err != nil {
		return "", "", fmt.Errorf("resolve log file: %w", err)
	}
	absPID, err := filepath.Abs(pidFile)
	if err != nil {
		return "", "", fmt.Errorf("resolve pid file: %w", err)
	}
	return absLog, absPID, nil
}

func isDaemonChild() bool {
	return os.Getenv(daemonChildEnv) == "1"
}

func startDaemonProcess(logFile, pidFile string) error {
	if err := os.MkdirAll(filepath.Dir(logFile), 0o755); err != nil {
		return fmt.Errorf("create log dir: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(pidFile), 0o755); err != nil {
		return fmt.Errorf("create pid dir: %w", err)
	}

	logHandle, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open log file: %w", err)
	}
	defer logHandle.Close()

	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve executable: %w", err)
	}

	childArgs := stripDaemonFlag(os.Args[1:])
	cmd := exec.Command(executable, childArgs...)
	detachDaemonProcess(cmd)
	cmd.Stdout = logHandle
	cmd.Stderr = logHandle
	cmd.Stdin = nil
	cmd.Env = append(os.Environ(),
		daemonChildEnv+"=1",
		daemonLogFileEnv+"="+logFile,
		daemonPIDFileEnv+"="+pidFile,
	)

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start daemon: %w", err)
	}
	if err := writePIDFile(pidFile, cmd.Process.Pid); err != nil {
		return fmt.Errorf("write pid file: %w", err)
	}

	fmt.Printf("cva daemon started\npid: %d\nlog: %s\npid file: %s\n", cmd.Process.Pid, logFile, pidFile)
	return nil
}

func writePIDFile(path string, pid int) error {
	return os.WriteFile(path, []byte(strconv.Itoa(pid)+"\n"), 0o644)
}

func removePIDFileIfOwned(path string, pid int) {
	content, err := os.ReadFile(path)
	if err != nil {
		return
	}
	recorded, err := strconv.Atoi(strings.TrimSpace(string(content)))
	if err != nil || recorded != pid {
		return
	}
	_ = os.Remove(path)
}

func stripDaemonFlag(args []string) []string {
	filtered := make([]string, 0, len(args))
	for _, arg := range args {
		if arg == "--daemon" {
			continue
		}
		filtered = append(filtered, arg)
	}
	return filtered
}

func cmdLogs(ctx context.Context, args []string) error {
	opts, err := parseLogsArgs(args)
	if err != nil {
		return err
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	logFile, _, err := resolveDaemonFiles(cfg, opts.logFile, "")
	if err != nil {
		return err
	}

	data, err := os.ReadFile(logFile)
	if err != nil {
		return fmt.Errorf("read log file %s: %w", logFile, err)
	}
	text := tailText(string(data), opts.lines)
	if text != "" {
		fmt.Print(text)
		if !strings.HasSuffix(text, "\n") {
			fmt.Println()
		}
	}

	if !opts.follow {
		return nil
	}
	return followLogFile(ctx, logFile)
}

func tailText(text string, limit int) string {
	trimmed := strings.TrimRight(text, "\n")
	if trimmed == "" || limit <= 0 {
		return ""
	}
	lines := strings.Split(trimmed, "\n")
	if len(lines) > limit {
		lines = lines[len(lines)-limit:]
	}
	return strings.Join(lines, "\n") + "\n"
}

func followLogFile(ctx context.Context, path string) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open log file %s: %w", path, err)
	}
	defer file.Close()

	offset, err := file.Seek(0, io.SeekEnd)
	if err != nil {
		return fmt.Errorf("seek log file: %w", err)
	}

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			stat, err := file.Stat()
			if err != nil {
				return fmt.Errorf("stat log file: %w", err)
			}
			if stat.Size() < offset {
				if _, err := file.Seek(0, io.SeekStart); err != nil {
					return fmt.Errorf("reset log file offset: %w", err)
				}
				offset = 0
			}
			if stat.Size() == offset {
				continue
			}
			if _, err := file.Seek(offset, io.SeekStart); err != nil {
				return fmt.Errorf("seek log file: %w", err)
			}
			written, err := io.Copy(os.Stdout, file)
			if err != nil {
				return fmt.Errorf("stream log file: %w", err)
			}
			offset += written
		}
	}
}

func cmdStop(args []string) error {
	pidFileOverride, err := parseStopArgs(args)
	if err != nil {
		return err
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	_, pidFile, err := resolveDaemonFiles(cfg, "", pidFileOverride)
	if err != nil {
		return err
	}

	content, err := os.ReadFile(pidFile)
	if err != nil {
		return fmt.Errorf("read pid file %s: %w", pidFile, err)
	}

	pid, err := strconv.Atoi(strings.TrimSpace(string(content)))
	if err != nil {
		return fmt.Errorf("parse pid file %s: %w", pidFile, err)
	}

	process, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("find process %d: %w", pid, err)
	}
	if err := process.Signal(os.Interrupt); err != nil && !errors.Is(err, os.ErrProcessDone) {
		return fmt.Errorf("signal process %d: %w", pid, err)
	}

	fmt.Printf("sent interrupt signal to cva daemon (pid %d)\n", pid)
	return nil
}
