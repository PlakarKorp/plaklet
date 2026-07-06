package main

import (
	"os"
	"runtime"
	"time"

	"github.com/shirou/gopsutil/v4/process"
)

// resourceSampler tracks this plaklet process's own CPU and memory over time.
// One plaklet process serves one job, so process-level usage is the job's usage.
//
// It self-decimates to stay bounded: once it fills to MaxResourceSamples it
// halves resolution (keeping every other sample) and doubles the stride, so the
// buffer spans the whole run at ~a few KB regardless of duration. Samples carry
// absolute timestamps since spacing becomes non-uniform.
type resourceSampler struct {
	proc    *process.Process
	samples []ResourceSample
	stride  int
	tick    int
}

func newResourceSampler() *resourceSampler {
	// NewProcess only fails for a missing pid, impossible for our own; fall back
	// to nil so sampling becomes a no-op rather than a hard failure.
	proc, _ := process.NewProcess(int32(os.Getpid()))
	return &resourceSampler{proc: proc, stride: 1}
}

func (r *resourceSampler) sample() {
	if r.proc == nil {
		return
	}
	// Percent(0) reports usage since the previous call; read every tick (not
	// just on store) so CPU% stays accurate regardless of stride.
	cpu, err := r.proc.Percent(0)
	if err != nil {
		return
	}
	var rss uint64
	if mem, err := r.proc.MemoryInfo(); err == nil && mem != nil {
		rss = mem.RSS
	}
	r.record(ResourceSample{
		At:          time.Now().UnixMilli(),
		CPUPercent:  cpu,
		MemoryBytes: rss,
	})
}

func (r *resourceSampler) record(s ResourceSample) {
	r.tick++
	if r.tick < r.stride {
		return
	}
	r.tick = 0

	r.samples = append(r.samples, s)
	if len(r.samples) > MaxResourceSamples {
		r.decimate()
	}
}

func (r *resourceSampler) decimate() {
	kept := make([]ResourceSample, 0, len(r.samples)/2+1)
	for i := 0; i < len(r.samples); i += 2 {
		kept = append(kept, r.samples[i])
	}
	r.samples = kept
	r.stride *= 2
}

// numCPU is the ceiling the whole-process CPUPercent is read against.
func (r *resourceSampler) numCPU() int { return runtime.NumCPU() }

func (r *resourceSampler) snapshot() []ResourceSample {
	if len(r.samples) == 0 {
		return nil
	}
	out := make([]ResourceSample, len(r.samples))
	copy(out, r.samples)
	return out
}
