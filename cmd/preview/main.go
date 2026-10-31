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
	"github.com/OpeniPod/OpeniPod/internal/desktop"
	"github.com/OpeniPod/OpeniPod/internal/library"
	"github.com/OpeniPod/OpeniPod/internal/player"
	"github.com/OpeniPod/OpeniPod/internal/storage"
)

type options struct {
	musicDir     string
	settingsFile string
	scale        int
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		slog.Error("preview stopped", "error", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	opts, err := parseOptions(args)
	if errors.Is(err, flag.ErrHelp) {
		return nil
	}
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return desktop.Run(opts.scale, func(window *desktop.Window) error {
		audio, err := player.NewPlayer()
		if err != nil {
			return fmt.Errorf("create audio player: %w", err)
		}
		return runApplication(ctx, opts, audio, window, window)
	})
}

func parseOptions(args []string) (options, error) {
	var opts options
	flags := flag.NewFlagSet("preview", flag.ContinueOnError)
	flags.StringVar(&opts.musicDir, "music-dir", "", "music directory (default: $HOME/Music)")
	flags.StringVar(&opts.settingsFile, "settings-file", "", "settings JSON path (default: user config directory/openipod/settings.json)")
	flags.IntVar(&opts.scale, "scale", 2, "initial window scale (1..4; display resolution is 240x320)")
	if err := flags.Parse(args); err != nil {
		return opts, err
	}
	if flags.NArg() != 0 {
		return opts, fmt.Errorf("unexpected argument %q", flags.Arg(0))
	}
	if opts.scale < 1 || opts.scale > 4 {
		return opts, fmt.Errorf("window scale must be between 1 and 4")
	}
	if opts.musicDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return opts, fmt.Errorf("resolve music directory: %w", err)
		}
		opts.musicDir = filepath.Join(home, "Music")
	}
	if opts.settingsFile == "" {
		config, err := os.UserConfigDir()
		if err != nil {
			return opts, fmt.Errorf("resolve settings directory: %w", err)
		}
		opts.settingsFile = filepath.Join(config, "openipod", "settings.json")
	}
	return opts, nil
}

// Tests supply a fake player and frontend; normal startup uses real audio and
// the native window. App.Run owns player shutdown in both cases.
func runApplication(ctx context.Context, opts options, audio app.Player, input app.Input, renderer app.Renderer) error {
	return app.New(audio, library.NewFilesystem(opts.musicDir),
		storage.NewFile(opts.settingsFile), input, renderer).Run(ctx)
}
