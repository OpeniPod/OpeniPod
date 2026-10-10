package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/OpeniPod/OpeniPod/internal/app"
	"github.com/OpeniPod/OpeniPod/internal/input"
	"github.com/OpeniPod/OpeniPod/internal/library"
	"github.com/OpeniPod/OpeniPod/internal/player"
	"github.com/OpeniPod/OpeniPod/internal/storage"
	"github.com/OpeniPod/OpeniPod/internal/ui"
)

func main() {
	if err := run(); err != nil {
		slog.Error("player stopped", "error", err)
		os.Exit(1)
	}
}

func run() error {
	flags := flag.NewFlagSet("player", flag.ContinueOnError)
	platform := flags.String("platform", "desktop", "platform backend (desktop)")
	settingsFile := flags.String("settings-file", "", "settings JSON path (default: user config directory/openipod/settings.json)")
	if err := flags.Parse(os.Args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if *platform != "desktop" {
		return fmt.Errorf("unsupported platform %q (available: desktop)", *platform)
	}
	if *settingsFile == "" {
		configDir, err := os.UserConfigDir()
		if err != nil {
			return fmt.Errorf("resolve settings directory: %w", err)
		}
		*settingsFile = filepath.Join(configDir, "openipod", "settings.json")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	application := app.New(
		player.NewFake(),
		library.NewFake(),
		storage.NewFile(*settingsFile),
		input.NewKeyboard(os.Stdin),
		ui.NewTerminal(os.Stdout),
	)
	return application.Run(ctx)
}
