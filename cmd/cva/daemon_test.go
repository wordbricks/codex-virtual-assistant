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

func TestWebURLForHTTPAddr(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		addr string
		want string
	}{
		{
			name: "default bind address",
			addr: "127.0.0.1:8080",
			want: "http://127.0.0.1:8080",
		},
		{
			name: "wildcard bind address uses localhost",
			addr: "0.0.0.0:9000",
			want: "http://127.0.0.1:9000",
		},
		{
			name: "scheme preserved",
			addr: "https://0.0.0.0:9443/app",
			want: "https://127.0.0.1:9443/app",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := webURLForHTTPAddr(tc.addr); got != tc.want {
				t.Fatalf("webURLForHTTPAddr(%q) = %q, want %q", tc.addr, got, tc.want)
			}
		})
	}
}
