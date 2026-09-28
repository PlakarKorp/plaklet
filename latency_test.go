package plaklet

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestLatencySamplerScopeMapping(t *testing.T) {
	io := map[string]IOScope{
		"storage":     {Write: IODir{Latency: Latency{Count: 10, Avg: time.Millisecond}}},
		"destination": {Write: IODir{Latency: Latency{Count: 7, Avg: time.Second}}},
	}

	for op, want := range map[string]int{"backup": 1, "sync": 1, "restore": 0, "check": 0} {
		l := newLatencySampler(op)
		l.sample(io)
		got := l.snapshot()
		require.Len(t, got, want, op)
		if want > 0 {
			require.Equal(t, int64(10), got[0].Count, op)
			require.Equal(t, time.Millisecond, got[0].Avg, op)
		}
	}
}

// An empty series means "not measured"; a zero reading would say "fast".
func TestLatencySamplerSkipsUnobserved(t *testing.T) {
	l := newLatencySampler("backup")
	l.sample(map[string]IOScope{"storage": {}})
	l.sample(nil)
	require.Nil(t, l.snapshot())
}

// kloset does not observe failed writes, so a stalled backend freezes Count.
// The series keeps the frozen points instead of dropping them.
func TestLatencySamplerKeepsStalledCount(t *testing.T) {
	io := map[string]IOScope{"storage": {Write: IODir{Latency: Latency{Count: 3}}}}
	l := newLatencySampler("backup")
	for range 3 {
		l.sample(io)
	}
	got := l.snapshot()
	require.Len(t, got, 3)
	for _, s := range got {
		require.Equal(t, int64(3), s.Count)
	}
}

func TestLatencySamplerDecimatesAndStaysBounded(t *testing.T) {
	l := newLatencySampler("backup")
	for i := 0; i < MaxLatencySamples*4; i++ {
		l.record(LatencySample{At: int64(i)})
	}
	require.LessOrEqual(t, len(l.samples), MaxLatencySamples)
	require.Greater(t, l.stride, 1)
	require.Equal(t, int64(0), l.samples[0].At)
}

func TestIODirFromEventDecodesLatency(t *testing.T) {
	e := ev("iostats", map[string]any{
		"w": map[string]any{
			"total": int64(4096),
			"latency": map[string]any{
				"count": int64(12),
				"avg":   2 * time.Millisecond,
				"p50":   time.Millisecond,
				"p95":   8 * time.Millisecond,
				"max":   30 * time.Millisecond,
			},
		},
	})
	require.Equal(t, Latency{
		Count: 12,
		Avg:   2 * time.Millisecond,
		P50:   time.Millisecond,
		P95:   8 * time.Millisecond,
		Max:   30 * time.Millisecond,
	}, ioDirFromEvent(e, "w").Latency)
}

func TestIODirFromEventWithoutLatency(t *testing.T) {
	e := ev("iostats", map[string]any{"r": map[string]any{"total": int64(512)}})
	require.Equal(t, Latency{}, ioDirFromEvent(e, "r").Latency)
}
