package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/OpeniPod/OpeniPod/internal/app"
	"github.com/OpeniPod/OpeniPod/internal/input"
	"github.com/OpeniPod/OpeniPod/internal/library"
	"github.com/OpeniPod/OpeniPod/internal/logging"
	"github.com/OpeniPod/OpeniPod/internal/player"
	"github.com/OpeniPod/OpeniPod/internal/storage"
	"github.com/OpeniPod/OpeniPod/internal/ui"
)

func main() {
	if err := run(os.Args[1:], os.Stdin, os.Stdout); err != nil {
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

func run(args []string, in *os.File, out io.Writer) error {
	return runWith(args, in, out, os.Stderr)
}

func runWith(args []string, stdin *os.File, stdout, stderr io.Writer) (result error) {
	flags := flag.NewFlagSet("player", flag.ContinueOnError)
	flags.SetOutput(stderr)
	platform := flags.String("platform", "desktop", "platform backend (desktop)")
	musicDir := flags.String("music-dir", "", "music directory with a saved library index (empty: sample library)")
	logTarget := flags.String("log-target", "file", "diagnostic output (file, journal)")
	logFile := flags.String("log-file", "", "log file path (default: XDG state directory)")
	logLevel := flags.String("log-level", "info", "diagnostic level (debug, info, warn, error)")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected arguments: %v", flags.Args())
	}
	if *platform != "desktop" {
		return fmt.Errorf("unsupported platform %q (available: desktop)", *platform)
	}
	logger, closeLog, err := logging.Open(*logTarget, *logFile, *logLevel, stderr)
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
	var musicLibrary app.Library = library.NewFake()
	if *musicDir != "" {
		musicLibrary = library.NewFilesystem(*musicDir)
	}
	application := app.New(
		player.NewFake(),
		musicLibrary,
		storage.NewMemory(50),
		input.NewKeyboard(stdin),
		ui.NewTerminal(stdout),
		app.WithLogger(logger),
	)
	if err := application.Run(ctx); err != nil {
		return &loggedError{err}
	}
	return nil
}
