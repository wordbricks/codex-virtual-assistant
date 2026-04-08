package main

import (
	"reflect"
	"testing"
)

func TestParseStartArgs(t *testing.T) {
	t.Parallel()

	opts, err := parseStartArgs([]string{
		"--yolo",
		"--daemon",
		"--log-file", "var/log/cva.log",
		"--pid-file=run/cva.pid",
	})
	if err != nil {
		t.Fatalf("parseStartArgs() error = %v", err)
	}

	if !opts.yolo {
		t.Fatal("parseStartArgs() did not set yolo")
	}
	if !opts.daemon {
		t.Fatal("parseStartArgs() did not set daemon")
	}
	if opts.logFile != "var/log/cva.log" {
		t.Fatalf("logFile = %q, want %q", opts.logFile, "var/log/cva.log")
	}
	if opts.pidFile != "run/cva.pid" {
		t.Fatalf("pidFile = %q, want %q", opts.pidFile, "run/cva.pid")
	}
}

func TestStripDaemonFlag(t *testing.T) {
	t.Parallel()

	args := []string{"--addr", "http://127.0.0.1:8080", "start", "--daemon", "--yolo"}
	got := stripDaemonFlag(args)
	want := []string{"--addr", "http://127.0.0.1:8080", "start", "--yolo"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("stripDaemonFlag() = %#v, want %#v", got, want)
	}
}

func TestTailText(t *testing.T) {
	t.Parallel()

	text := "one\ntwo\nthree\nfour\n"
	got := tailText(text, 2)
	want := "three\nfour\n"
	if got != want {
		t.Fatalf("tailText() = %q, want %q", got, want)
	}
}
