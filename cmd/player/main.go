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
	"github.com/OpeniPod/OpeniPod/internal/desktop"
	"github.com/OpeniPod/OpeniPod/internal/library"
	"github.com/OpeniPod/OpeniPod/internal/logging"
	"github.com/OpeniPod/OpeniPod/internal/player"
	"github.com/OpeniPod/OpeniPod/internal/storage"
)

type options struct {
	platform     string
	musicDir     string
	settingsFile string
	logTarget    string
	logFile      string
	logLevel     string
	scale        int
}

func main() {
	if err := run(os.Args[1:], os.Stderr, launchDesktop); err != nil {
		var recorded *loggedError
		if !errors.As(err, &recorded) {
			fmt.Fprintln(os.Stderr, err)
		}
		os.Exit(1)
	}
}

// loggedError prevents main from writing a fatal runtime error to the journal
// a second time. Configuration errors before App.Run still reach stderr.
type loggedError struct{ error }

func (e *loggedError) Unwrap() error { return e.error }

func run(args []string, stderr io.Writer, launch func(context.Context, options, *slog.Logger) error) (result error) {
	opts, err := parseOptions(args, stderr)
	if errors.Is(err, flag.ErrHelp) {
		return nil
	}
	if err != nil {
		return err
	}
	logger, closeLog, err := logging.Open(opts.logTarget, opts.logFile, opts.logLevel, stderr)
	if err != nil {
		return err
	}
	defer func() {
		if err := closeLog(); err != nil && result == nil {
			result = fmt.Errorf("close log: %w", err)
		}
	}()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return launch(ctx, opts, logger)
}

func parseOptions(args []string, stderr io.Writer) (options, error) {
	var opts options
	flags := flag.NewFlagSet("player", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.StringVar(&opts.platform, "platform", "desktop", "platform backend (desktop)")
	flags.StringVar(&opts.musicDir, "music-dir", "", "music directory with a saved library index (default: $HOME/Music)")
	flags.StringVar(&opts.settingsFile, "settings-file", "", "settings JSON path (default: user config directory/openipod/settings.json)")
	flags.StringVar(&opts.logTarget, "log-target", "file", "diagnostic output (file, journal)")
	flags.StringVar(&opts.logFile, "log-file", "", "log file path (default: XDG state directory)")
	flags.StringVar(&opts.logLevel, "log-level", "info", "diagnostic level (debug, info, warn, error)")
	flags.IntVar(&opts.scale, "scale", 2, "initial window scale (1..4; display resolution is 240x320)")
	if err := flags.Parse(args); err != nil {
		return opts, err
	}
	if flags.NArg() != 0 {
		return opts, fmt.Errorf("unexpected arguments: %v", flags.Args())
	}
	if opts.platform != "desktop" {
		return opts, fmt.Errorf("unsupported platform %q (available: desktop)", opts.platform)
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
		configDir, err := os.UserConfigDir()
		if err != nil {
			return opts, fmt.Errorf("resolve settings directory: %w", err)
		}
		opts.settingsFile = filepath.Join(configDir, "openipod", "settings.json")
	}
	return opts, nil
}

func launchDesktop(ctx context.Context, opts options, logger *slog.Logger) error {
	return desktop.Run(opts.scale, func(window *desktop.Window) error {
		audio, err := player.NewPlayer()
		if err != nil {
			return fmt.Errorf("create audio player: %w", err)
		}
		return runApplication(ctx, opts, audio, window, window, logger)
	})
}

// App.Run owns player shutdown. Tests supply a fake player and frontend.
func runApplication(ctx context.Context, opts options, audio app.Player, input app.Input, renderer app.Renderer, logger *slog.Logger) error {
	application := app.New(audio, library.NewFilesystem(opts.musicDir),
		storage.NewFile(opts.settingsFile), input, renderer, app.WithLogger(logger))
	if err := application.Run(ctx); err != nil {
		return &loggedError{err}
	}
	return nil
}
