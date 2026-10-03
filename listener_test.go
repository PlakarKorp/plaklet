package plaklet

import (
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"testing"

	"github.com/PlakarKorp/kloset/events"
	"github.com/google/uuid"
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
