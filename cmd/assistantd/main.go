package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/siisee11/CodexVirtualAssistant/internal/app"
	"github.com/siisee11/CodexVirtualAssistant/internal/config"
)

type startupOptions struct {
	yolo bool
}

func main() {
	options, err := parseStartupOptions(os.Args[1:])
	if err != nil {
		log.Fatalf("parse startup options: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	cfg = applyStartupOptions(cfg, options)
	if options.yolo {
		log.Printf("assistantd starting with --yolo; Codex sandbox forced to %s", cfg.CodexSandboxMode)
	}

	application, err := app.New(cfg)
	if err != nil {
		log.Fatalf("bootstrap app: %v", err)
	}

	log.Printf("assistantd listening on %s", cfg.HTTPAddr)
	if err := application.Run(ctx); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("run app: %v", err)
	}
}

func parseStartupOptions(args []string) (startupOptions, error) {
	var options startupOptions
	for _, arg := range args {
		switch arg {
		case "--yolo":
			options.yolo = true
		default:
			return startupOptions{}, fmt.Errorf("unknown argument %q", arg)
		}
	}
	return options, nil
}

func applyStartupOptions(cfg config.Config, options startupOptions) config.Config {
	if options.yolo {
		cfg.CodexSandboxMode = "danger-full-access"
	}
	return cfg
}
