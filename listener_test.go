package plaklet

import (
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/PlakarKorp/kloset/events"
	"github.com/PlakarKorp/kloset/objects"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// The state sampler encodes the listener state from its own goroutine while
// events keep arriving. Run under -race.
func TestEventListenerStateIsSafeToEncodeWhileEventsArrive(t *testing.T) {
	bus := events.NewEventsBUS(64)

	listener := NewEventListener(true)
	listener.Run(bus)

	emitting := make(chan struct{})

	var wg sync.WaitGroup
	wg.Go(func() {
		defer close(emitting)
		defer bus.Close()

		emitter := bus.NewRepositoryEmitter(uuid.Must(uuid.NewRandom()), "backup")
		for i := range 500 {
			emitter.Info("iostats", map[string]any{
				"scope": fmt.Sprintf("scope-%d", i),
				"r":     map[string]any{"total": float64(i)},
				"w":     map[string]any{"total": float64(i)},
			})
			emitter.Path(fmt.Sprintf("/path-%d", i))
			emitter.PathOk(fmt.Sprintf("/path-%d", i))
		}
	})

	enc := json.NewEncoder(io.Discard)
	for {
		select {
		case <-emitting:
			wg.Wait()
			listener.Wait()
			if got := len(listener.State().IO); got != 500 {
				t.Fatalf("IO scopes = %d, want 500", got)
			}
			return
		default:
			state := listener.State()
			if err := enc.Encode(state); err != nil {
				t.Fatalf("encode state: %s", err)
			}
			if !state.Equal(listener.State()) {
				continue
			}
		}
	}
}

// Processed counts what was actually done. The scan summary is only the total,
// so a restore whose exporter never wrote a file reads as zero, not as done.
func TestEventListenerProcessed(t *testing.T) {
	tests := []struct {
		name      string
		emit      func(e *events.Emitter)
		wantItems uint64
		wantBytes uint64
	}{
		{
			name: "summary only",
			emit: func(e *events.Emitter) { e.FilesystemSummary(3, 1, 0, 0, 24) },
		},
		{
			name: "acked files",
			emit: func(e *events.Emitter) {
				e.FilesystemSummary(3, 1, 0, 0, 24)
				e.FileOk("/a", objects.NewFileInfo("a", 8, 0o644, time.Time{}, 0, 0, 0, 0, 0))
			},
			wantItems: 1,
			wantBytes: 8,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bus := events.NewEventsBUS(64)
			listener := NewEventListener(true)
			listener.Run(bus)

			tt.emit(bus.NewRepositoryEmitter(uuid.Must(uuid.NewRandom()), "restore"))
			bus.Close()
			listener.Wait()

			st := listener.State()
			require.Equal(t, tt.wantItems, st.Processed.Items)
			require.Equal(t, tt.wantBytes, st.Processed.Bytes)
		})
	}
}
