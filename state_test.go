package plaklet

import (
	"testing"
	"time"

	"github.com/PlakarKorp/kloset/events"
	"github.com/PlakarKorp/kloset/objects"
	"github.com/stretchr/testify/require"
)

func ev(typ string, data map[string]any) events.Event {
	return events.Event{Type: typ, Data: data}
}

func TestUpdateStateCounters(t *testing.T) {
	var s State
	updateState(&s, ev("file", nil))
	updateState(&s, ev("file.ok", map[string]any{
		"fileinfo": objects.NewFileInfo("a", 100, 0, time.Time{}, 0, 0, 0, 0, 0),
	}))
	updateState(&s, ev("file.cached", map[string]any{
		"fileinfo": objects.NewFileInfo("b", 50, 0, time.Time{}, 0, 0, 0, 0, 0),
	}))
	updateState(&s, ev("directory", nil))
	updateState(&s, ev("directory.ok", nil))

	require.Equal(t, uint64(1), s.Files.Total)
	require.Equal(t, uint64(1), s.Files.Ok)
	require.Equal(t, uint64(1), s.Files.Cached)
	require.Equal(t, uint64(100), s.Files.Size)
	require.Equal(t, uint64(50), s.Files.CachedSize)
	require.Equal(t, uint64(1), s.Dirs.Total)
	require.Equal(t, uint64(1), s.Dirs.Ok)
}

func TestUpdateStatePhaseAndResult(t *testing.T) {
	var s State
	updateState(&s, ev("snapshot.import.start", nil))
	require.Equal(t, "processing", s.Phase)

	updateState(&s, ev("result", map[string]any{
		"size":     uint64(2048),
		"errors":   uint64(1),
		"duration": 3 * time.Second,
	}))
	require.Equal(t, "completed", s.Phase)
	require.Equal(t, uint64(2048), s.Result.Size)
	require.Equal(t, uint64(1), s.Result.Errors)
	require.Equal(t, 3.0, s.Result.Duration)
}

func TestUpdateStateIostats(t *testing.T) {
	var s State
	updateState(&s, ev("iostats", map[string]any{
		"scope": "source",
		"r":     map[string]any{"total": int64(1000), "overall": 2000.0, "overall_wall": 3000.0},
		"w":     map[string]any{"total": int64(0)},
	}))
	require.Contains(t, s.IO, "source")
	require.Equal(t, int64(1000), s.IO["source"].Read.TotalBytes)
	require.Equal(t, 2000.0, s.IO["source"].Read.Overall)
	require.Equal(t, 3000.0, s.IO["source"].Read.OverallWall)
}

func TestUpdateStateIostatsIgnoresMissingScope(t *testing.T) {
	var s State
	updateState(&s, ev("iostats", map[string]any{"r": map[string]any{"total": int64(5)}}))
	require.Nil(t, s.IO)
}

// processed() prefers per-file counters, falling back to the scan summary.
func TestProcessedFallback(t *testing.T) {
	// Counters present -> use them.
	s := State{}
	s.Files.Ok, s.Files.Size = 3, 300
	s.Files.Cached, s.Files.CachedSize = 2, 200
	items, bytes := s.processed()
	require.Equal(t, uint64(5), items)
	require.Equal(t, uint64(500), bytes)

	// No counters -> fall back to summary.
	s2 := State{}
	s2.Summary.Files, s2.Summary.Size = 7, 700
	items, bytes = s2.processed()
	require.Equal(t, uint64(7), items)
	require.Equal(t, uint64(700), bytes)
}

func TestEventFieldTypeMismatch(t *testing.T) {
	e := ev("x", map[string]any{"n": "not-an-int"})
	_, ok := eventField[int64](e, "n")
	require.False(t, ok)
	_, ok = eventField[int64](e, "missing")
	require.False(t, ok)
}

func TestIODirFromEventCoercesNumbers(t *testing.T) {
	e := ev("iostats", map[string]any{
		"r": map[string]any{"total": int64(10), "overall": 1.5},
	})
	d := ioDirFromEvent(e, "r")
	require.Equal(t, int64(10), d.TotalBytes)
	require.Equal(t, 1.5, d.Overall)
	require.Equal(t, 0.0, d.OverallWall) // absent -> zero
}
