package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
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
	if err := run(os.Args[1:], os.Stdin, os.Stdout); err != nil {
		slog.Error("player stopped", "error", err)
		os.Exit(1)
	}
}

func run(args []string, in *os.File, out io.Writer) error {
	return runWithPlayerFactory(args, in, out, func() (app.Player, error) {
		return player.NewPlayer()
	})
}

func runWithPlayerFactory(args []string, in *os.File, out io.Writer, newPlayer func() (app.Player, error)) error {
	flags := flag.NewFlagSet("player", flag.ContinueOnError)
	platform := flags.String("platform", "desktop", "platform backend (desktop)")
	musicDir := flags.String("music-dir", "", "music directory with a saved library index (empty: sample library)")
	settingsFile := flags.String("settings-file", "", "settings JSON path (default: user config directory/openipod/settings.json)")
	if err := flags.Parse(args); err != nil {
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

	audioPlayer, err := newPlayer()
	if err != nil {
		return fmt.Errorf("create audio player: %w", err)
	}

	var musicLibrary app.Library = library.NewFake()

	if *musicDir != "" {
		musicLibrary = library.NewFilesystem(*musicDir)
	}

	application := app.New(
		audioPlayer,
		musicLibrary,
		storage.NewFile(*settingsFile),
		input.NewKeyboard(in),
		ui.NewTerminal(out),
	)

	return application.Run(ctx)
}
