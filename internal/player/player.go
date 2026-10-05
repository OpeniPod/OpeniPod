package player

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/OpeniPod/OpeniPod/internal/app"
	"github.com/OpeniPod/OpeniPod/internal/music"

	"github.com/gopxl/beep"
	"github.com/gopxl/beep/effects"
	"github.com/gopxl/beep/mp3"
	"github.com/gopxl/beep/speaker"
)

const sampleRate beep.SampleRate = 48000
const positionInterval = 250 * time.Millisecond

var ErrClosed = errors.New("player is closed")

type Player struct {
	mu         sync.Mutex
	events     chan app.Event
	eventMu    sync.Mutex
	eventQueue []app.Event
	eventReady chan struct{}
	eventWG    sync.WaitGroup
	shutdown   chan struct{}
	closeDone  chan struct{}
	closeErr   error
	closed     bool
	callbackWG sync.WaitGroup

	streamer beep.StreamSeekCloser
	ctrl     *beep.Ctrl
	volume   *effects.Volume

	track       music.Track
	volumeLevel int
	playbackID  uint64
}

func NewPlayer() (*Player, error) {
	if err := speaker.Init(sampleRate, sampleRate.N(time.Second/10)); err != nil {
		return nil, fmt.Errorf("initialize audio speaker: %w", err)
	}

	p := &Player{
		events:      make(chan app.Event, 16),
		eventReady:  make(chan struct{}, 1),
		shutdown:    make(chan struct{}),
		closeDone:   make(chan struct{}),
		volumeLevel: 50,
	}
	p.eventWG.Add(1)
	go p.dispatchEvents()
	return p, nil
}

func (p *Player) Play(ctx context.Context, track music.Track) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	file, err := os.Open(track.Path)
	if err != nil {
		return p.reportPlaybackError(ctx, fmt.Errorf("open track %q: %w", track.Path, err))
	}

	streamer, format, err := mp3.Decode(file)
	if err != nil {
		decodeErr := errors.Join(fmt.Errorf("decode track %q: %w", track.Path, err), file.Close())
		return p.reportPlaybackError(ctx, decodeErr)
	}

	if err := ctx.Err(); err != nil {
		return errors.Join(err, streamer.Close())
	}

	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return errors.Join(ErrClosed, streamer.Close())
	}
	if err := ctx.Err(); err != nil {
		p.mu.Unlock()
		return errors.Join(err, streamer.Close())
	}

	speaker.Clear()
	p.playbackID++
	playbackID := p.playbackID
	if p.streamer != nil {
		if err := p.streamer.Close(); err != nil {
			p.streamer = nil
			p.ctrl = nil
			p.volume = nil
			p.track = music.Track{}
			p.mu.Unlock()

			if newErr := streamer.Close(); newErr != nil {
				return errors.Join(
					fmt.Errorf("close previous track: %w", err),
					fmt.Errorf("close new track: %w", newErr),
				)
			}
			return fmt.Errorf("close previous track: %w", err)
		}
	}

	p.streamer = streamer
	p.track = track

	resampled := beep.Resample(4, format.SampleRate, sampleRate, streamer)

	p.ctrl = &beep.Ctrl{
		Streamer: resampled,
		Paused:   false,
	}

	p.volume = &effects.Volume{
		Streamer: p.ctrl,
		Base:     2,
		Volume:   volumeToBeep(p.volumeLevel),
		Silent:   p.volumeLevel == 0,
	}

	if err := ctx.Err(); err != nil {
		p.streamer = nil
		p.ctrl = nil
		p.volume = nil
		p.track = music.Track{}
		p.mu.Unlock()
		return errors.Join(err, streamer.Close())
	}
	p.queueEvent(app.PlaybackStarted{Track: track})

	completed := make(chan struct{})
	p.callbackWG.Add(1)
	go func() {
		defer p.callbackWG.Done()
		ticker := time.NewTicker(positionInterval)
		defer ticker.Stop()
		for {
			select {
			case <-completed:
				p.finishPlayback(playbackID, streamer)
				return
			case <-ticker.C:
				p.reportPosition(playbackID, streamer, format.SampleRate)
			case <-p.shutdown:
				return
			}
		}
	}()
	speaker.Play(beep.Seq(p.volume, beep.Callback(func() {
		close(completed)
	})))
	p.mu.Unlock()
	return nil
}

func (p *Player) reportPlaybackError(ctx context.Context, err error) error {
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return ErrClosed
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	if !p.queueEvent(app.PlaybackError{Err: err}) {
		return ErrClosed
	}
	return nil
}

func (p *Player) Close() error {
	p.mu.Lock()
	if p.closed {
		closeDone := p.closeDone
		p.mu.Unlock()
		<-closeDone
		return p.closeErr
	}

	p.closed = true
	close(p.shutdown)
	p.playbackID++
	streamer := p.streamer
	p.streamer = nil
	p.ctrl = nil
	p.volume = nil
	p.track = music.Track{}
	speaker.Clear()
	p.mu.Unlock()

	speaker.Close()
	var closeErr error
	if streamer != nil {
		closeErr = streamer.Close()
	}
	p.callbackWG.Wait()
	p.eventWG.Wait()

	if closeErr != nil {
		closeErr = fmt.Errorf("close current audio stream: %w", closeErr)
	}
	p.mu.Lock()
	p.closeErr = closeErr
	close(p.closeDone)
	p.mu.Unlock()
	return closeErr
}

func (p *Player) Pause(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return ErrClosed
	}
	if p.ctrl == nil {
		return errors.New("no track is playing")
	}

	wasPaused := p.ctrl.Paused
	speaker.Lock()
	p.ctrl.Paused = true
	speaker.Unlock()

	if err := ctx.Err(); err != nil {
		speaker.Lock()
		p.ctrl.Paused = wasPaused
		speaker.Unlock()
		return err
	}
	p.queueEvent(app.PlaybackPaused{})
	return nil
}

func (p *Player) Resume(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return ErrClosed
	}
	if p.ctrl == nil {
		return errors.New("no track is playing")
	}

	wasPaused := p.ctrl.Paused
	speaker.Lock()
	p.ctrl.Paused = false
	speaker.Unlock()

	if err := ctx.Err(); err != nil {
		speaker.Lock()
		p.ctrl.Paused = wasPaused
		speaker.Unlock()
		return err
	}
	p.queueEvent(app.PlaybackResumed{})
	return nil
}

func (p *Player) SetVolume(ctx context.Context, vol int) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return ErrClosed
	}
	if vol < 0 {
		vol = 0
	}

	if vol > 100 {
		vol = 100
	}

	p.volumeLevel = vol

	if p.volume != nil {
		speaker.Lock()
		p.volume.Volume = volumeToBeep(vol)
		p.volume.Silent = vol == 0
		speaker.Unlock()
	}

	return nil
}

func (p *Player) finishPlayback(playbackID uint64, streamer beep.StreamSeekCloser) {
	p.mu.Lock()
	if p.closed || p.playbackID != playbackID || p.streamer == nil {
		p.mu.Unlock()
		return
	}

	p.streamer = nil
	p.ctrl = nil
	p.volume = nil
	p.track = music.Track{}
	p.mu.Unlock()

	var event app.Event = app.TrackFinished{}
	if err := errors.Join(streamer.Err(), streamer.Close()); err != nil {
		event = app.PlaybackError{Err: fmt.Errorf("complete track: %w", err)}
	}
	p.queueEvent(event)
}

func (p *Player) reportPosition(playbackID uint64, streamer beep.StreamSeekCloser, rate beep.SampleRate) {
	p.mu.Lock()
	if p.closed || p.playbackID != playbackID || p.streamer == nil || p.ctrl == nil {
		p.mu.Unlock()
		return
	}

	speaker.Lock()
	paused := p.ctrl.Paused
	position := streamer.Position()
	speaker.Unlock()
	p.mu.Unlock()

	if paused {
		return
	}
	p.queueEvent(app.PlaybackPositionChanged{Position: rate.D(position)})
}

func (p *Player) queueEvent(event app.Event) bool {
	select {
	case <-p.shutdown:
		return false
	default:
	}

	p.eventMu.Lock()
	select {
	case <-p.shutdown:
		p.eventMu.Unlock()
		return false
	default:
	}
	p.eventQueue = append(p.eventQueue, event)
	p.eventMu.Unlock()

	select {
	case p.eventReady <- struct{}{}:
	default:
	}
	return true
}

func (p *Player) dispatchEvents() {
	defer p.eventWG.Done()
	for {
		p.eventMu.Lock()
		if len(p.eventQueue) > 0 {
			event := p.eventQueue[0]
			p.eventQueue[0] = nil
			p.eventQueue = p.eventQueue[1:]
			if len(p.eventQueue) == 0 {
				p.eventQueue = nil
			}
			p.eventMu.Unlock()

			select {
			case p.events <- event:
			case <-p.shutdown:
				return
			}
			continue
		}
		p.eventMu.Unlock()

		select {
		case <-p.eventReady:
		case <-p.shutdown:
			return
		}
	}
}

func volumeToBeep(vol int) float64 {
	// 0: -6.0, 25: -4.5, 50: -3.0, 75: -1.5, 100: 0.0
	return -6.0 + 6.0*float64(vol)/100.0
}

func (p *Player) Events() <-chan app.Event {
	return p.events
}
