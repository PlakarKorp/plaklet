package plaklet

import "time"

// latencySampler builds a write-latency series from the latest per-scope
// iostats. kloset only measures packfile writes to the storage backend, so it
// tracks the "storage" scope of the operations that write to one; the others
// produce no samples. Self-decimates like resourceSampler.
type latencySampler struct {
	scope string

	samples []LatencySample
	stride  int
	tick    int
}

func newLatencySampler(op string) *latencySampler {
	var scope string
	switch op {
	case "backup", "sync":
		scope = "storage"
	}
	return &latencySampler{scope: scope, stride: 1}
}

// sample skips a scope with no observation yet: an empty series means "not
// measured", which a zero reading would misreport as fast.
func (l *latencySampler) sample(io map[string]IOScope) {
	if l.scope == "" {
		return
	}

	lat := io[l.scope].Write.Latency
	if lat.Count == 0 {
		return
	}

	l.tick++
	if l.tick < l.stride {
		return
	}
	l.tick = 0

	l.record(LatencySample{
		At:    time.Now().UnixMilli(),
		Count: lat.Count,
		Avg:   lat.Avg,
		P50:   lat.P50,
		P95:   lat.P95,
		Max:   lat.Max,
	})
}

func (l *latencySampler) record(s LatencySample) {
	l.samples = append(l.samples, s)
	if len(l.samples) > MaxLatencySamples {
		kept := make([]LatencySample, 0, len(l.samples)/2+1)
		for i := 0; i < len(l.samples); i += 2 {
			kept = append(kept, l.samples[i])
		}
		l.samples = kept
		l.stride *= 2
	}
}

func (l *latencySampler) snapshot() []LatencySample {
	if len(l.samples) == 0 {
		return nil
	}
	out := make([]LatencySample, len(l.samples))
	copy(out, l.samples)
	return out
}
