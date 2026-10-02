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
	flags := flag.NewFlagSet("player", flag.ContinueOnError)
	platform := flags.String("platform", "desktop", "platform backend (desktop)")
	musicDir := flags.String("music-dir", "", "music directory with a saved library index (empty: sample library)")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if *platform != "desktop" {
		return fmt.Errorf("unsupported platform %q (available: desktop)", *platform)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	var musicLibrary app.Library = library.NewFake()
	if *musicDir != "" {
		musicLibrary = library.NewFilesystem(*musicDir)
	}
	application := app.New(
		player.NewFake(),
		musicLibrary,
		storage.NewMemory(50),
		input.NewKeyboard(in),
		ui.NewTerminal(out),
	)
	return application.Run(ctx)
}
