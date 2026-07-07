package main

import (
	"testing"

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
