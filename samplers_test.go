package plaklet

import (
	"testing"
	"time"

	"github.com/shirou/gopsutil/v4/process"
	"github.com/stretchr/testify/require"
)

func TestResourceSamplerDecimatesAndStaysBounded(t *testing.T) {
	r := &resourceSampler{stride: 1}
	// Feed well past the cap; the buffer must never exceed MaxResourceSamples and
	// the stride must grow as it decimates.
	for i := 0; i < MaxResourceSamples*4; i++ {
		r.record(ResourceSample{At: int64(i)})
	}
	require.LessOrEqual(t, len(r.samples), MaxResourceSamples)
	require.Greater(t, r.stride, 1, "stride should have grown from decimation")
	// The very first sample (t=0) is preserved across decimation.
	require.Equal(t, int64(0), r.samples[0].At)
}

func TestResourceSamplerSnapshotIsACopy(t *testing.T) {
	r := &resourceSampler{stride: 1}
	r.record(ResourceSample{At: 1})
	snap := r.snapshot()
	r.record(ResourceSample{At: 2})
	require.Len(t, snap, 1, "snapshot must not see later appends")
}

func TestResourceSamplerEmptySnapshotIsNil(t *testing.T) {
	require.Nil(t, (&resourceSampler{stride: 1}).snapshot())
}

func TestNetworkScopeResolution(t *testing.T) {
	cases := map[string][2]string{
		"backup":  {"source", "storage"},
		"restore": {"storage", "destination"},
		"check":   {"storage", ""},
		"sync":    {"source-storage", "storage"},
	}
	for op, want := range cases {
		n := newNetworkSampler(op)
		require.Equal(t, want[0], n.readScope, op)
		require.Equal(t, want[1], n.writeScope, op)
	}
}

func TestNetworkSamplerReadsCorrectDirections(t *testing.T) {
	n := newNetworkSampler("backup") // read=source (wall), write=storage (active)
	io := map[string]IOScope{
		"source":  {Read: IODir{OverallWall: 1500}},
		"storage": {Write: IODir{Overall: 800}},
	}
	n.sample(io)
	require.Len(t, n.samples, 1)
	require.Equal(t, 1500.0, n.samples[0].ReadBytesPerSec)
	require.Equal(t, 800.0, n.samples[0].WriteBytesPerSec)
}

func TestNetworkSamplerNoScopesNoSamples(t *testing.T) {
	n := &networkSampler{stride: 1} // both scopes empty (e.g. unknown op)
	n.sample(map[string]IOScope{"x": {Read: IODir{OverallWall: 1}}})
	require.Empty(t, n.samples)
}

func TestNetworkSamplerDecimatesAndStaysBounded(t *testing.T) {
	n := &networkSampler{stride: 1}
	for i := 0; i < MaxNetworkSamples*4; i++ {
		n.record(NetworkSample{At: int64(i)})
	}
	require.LessOrEqual(t, len(n.samples), MaxNetworkSamples)
	require.Greater(t, n.stride, 1)
	require.Equal(t, int64(0), n.samples[0].At)
}

func TestResourceSamplerReadsOwnProcess(t *testing.T) {
	r := newResourceSampler()
	require.NotNil(t, r.proc)
	r.sample()
	r.sample()
	require.Len(t, r.samples, 2)
	require.Positive(t, r.samples[1].MemoryBytes)
	require.Positive(t, r.numCPU())

	// Without a process handle sampling is a no-op.
	none := &resourceSampler{stride: 1}
	none.sample()
	require.Empty(t, none.samples)

	// A process that cannot be read yields nothing where the platform
	// reports the failure (Linux), and a zero reading where it does not
	// (macOS reads an unknown pid as idle); neither is a crash.
	gone := &resourceSampler{proc: &process.Process{Pid: 1<<31 - 1}, stride: 1}
	gone.sample()
	require.LessOrEqual(t, len(gone.samples), 1)
	for _, s := range gone.samples {
		require.Zero(t, s.CPUPercent)
	}
}

func TestNetworkSamplerHonoursStride(t *testing.T) {
	n := newNetworkSampler("check")
	n.stride = 2
	io := map[string]IOScope{"storage": {Read: IODir{OverallWall: 10}}}
	n.sample(io)
	require.Empty(t, n.samples, "first tick of a stride of two is skipped")
	require.Nil(t, n.snapshot())
	n.sample(io)
	require.Len(t, n.samples, 1)
	snap := n.snapshot()
	n.sample(io)
	n.sample(io)
	require.Len(t, snap, 1, "snapshot must not see later appends")
}

func TestLatencySamplerHonoursStride(t *testing.T) {
	l := newLatencySampler("backup")
	l.stride = 2
	io := map[string]IOScope{"storage": {Write: IODir{Latency: Latency{Count: 3, Avg: time.Millisecond}}}}
	l.sample(io)
	require.Empty(t, l.samples)
	l.sample(io)
	require.Len(t, l.samples, 1)
}
