package app_test

import (
	"context"

	"github.com/OpeniPod/OpeniPod/internal/app"
)

type eventInput struct {
	events []app.Event
	err    error
}

func (in eventInput) Run(ctx context.Context, out chan<- app.Event) error {
	for _, event := range in.events {
		select {
		case out <- event:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return in.err
}
