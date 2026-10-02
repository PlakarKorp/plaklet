package plaklet

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// fullState builds a State with every comparable field set to a non-zero value
// so mutations in tests exercise each clause of Equal.
func genFullState() State {
	s := State{
		Version:    "v1.0.0",
		SnapshotID: "snap-1",
		Phase:      "backup",
		NumCPU:     8,
		Resources:  []ResourceSample{{At: 1, CPUPercent: 50, MemoryBytes: 100}},
		Network:    []NetworkSample{{At: 2, ReadBytesPerSec: 10, WriteBytesPerSec: 20}},
		Latency:    []LatencySample{{At: 3, Count: 4, Avg: time.Millisecond, P50: time.Millisecond, P95: 2 * time.Millisecond, Max: 3 * time.Millisecond}},
		IO: map[string]IOScope{"source": {
			Read:  IODir{TotalBytes: 5},
			Write: IODir{Latency: Latency{Count: 2, Avg: time.Millisecond}},
		}},
		RecentPaths: []RecentPath{
			{Path: "/etc/passwd", Status: "ok"},
		},
	}
	s.Summary.Exists = true
	s.Summary.Paths = 3
	s.Summary.Files = 2
	s.Summary.Directories = 1
	s.Summary.Symlinks = 1
	s.Summary.Xattrs = 1
	s.Summary.Size = 4096

	s.Paths = StateCounter{Total: 10, Ok: 9, Error: 1, Size: 100}
	s.Dirs = StateCounter{Total: 5}
	s.Files = StateCounter{Total: 4}
	s.Xattrs = StateCounter{Total: 3}
	s.Symlinks = StateCounter{Total: 2}
	s.Chunks = StateCounter{Total: 1}
	s.Objects = StateCounter{Total: 7}

	s.Result.Size = 8192
	s.Result.Errors = 1
	s.Result.Duration = 1.5

	s.Processed.Items = 42
	s.Processed.Bytes = 1024
	return s
}

func TestGenStateEqual_Identical(t *testing.T) {
	a := genFullState()
	b := genFullState()
	require.True(t, a.Equal(b))
	require.True(t, b.Equal(a))
}

func TestGenStateEqual_ZeroValues(t *testing.T) {
	require.True(t, State{}.Equal(State{}))
}

func TestGenStateEqual_DifferingFields(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(s *State)
	}{
		{"Version", func(s *State) { s.Version = "v2.0.0" }},
		{"SnapshotID", func(s *State) { s.SnapshotID = "snap-2" }},
		{"Phase", func(s *State) { s.Phase = "restore" }},
		{"Summary", func(s *State) { s.Summary.Files = 999 }},
		{"Paths", func(s *State) { s.Paths.Ok = 0 }},
		{"Dirs", func(s *State) { s.Dirs.Total = 0 }},
		{"Files", func(s *State) { s.Files.Total = 0 }},
		{"Xattrs", func(s *State) { s.Xattrs.Total = 0 }},
		{"Symlinks", func(s *State) { s.Symlinks.Total = 0 }},
		{"Chunks", func(s *State) { s.Chunks.Total = 0 }},
		{"Objects", func(s *State) { s.Objects.Total = 0 }},
		{"Result", func(s *State) { s.Result.Duration = 9.9 }},
		{"NumCPU", func(s *State) { s.NumCPU = 4 }},
		{"Processed", func(s *State) { s.Processed.Bytes = 0 }},
		{"Resources_value", func(s *State) { s.Resources[0].CPUPercent = 1 }},
		{"Resources_len", func(s *State) { s.Resources = nil }},
		{"Network_value", func(s *State) { s.Network[0].ReadBytesPerSec = 1 }},
		{"Network_len", func(s *State) { s.Network = append(s.Network, NetworkSample{}) }},
		{"Latency_value", func(s *State) { s.Latency[0].P95 = time.Second }},
		{"Latency_count", func(s *State) { s.Latency[0].Count = 99 }},
		{"Latency_len", func(s *State) { s.Latency = nil }},
		{"IO_latency", func(s *State) {
			s.IO["source"] = IOScope{
				Read:  IODir{TotalBytes: 5},
				Write: IODir{Latency: Latency{Count: 2, Avg: time.Second}},
			}
		}},
		{"IO_value", func(s *State) { s.IO["source"] = IOScope{Read: IODir{TotalBytes: 999}} }},
		{"IO_key", func(s *State) { s.IO = map[string]IOScope{"storage": {}} }},
		{"IO_empty", func(s *State) { s.IO = nil }},
		{"RecentPaths_value", func(s *State) { s.RecentPaths[0].Status = "error" }},
		{"RecentPaths_len", func(s *State) { s.RecentPaths = nil }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := genFullState()
			b := genFullState()
			tt.mutate(&b)
			require.False(t, a.Equal(b), "expected inequality after mutating %s", tt.name)
			require.False(t, b.Equal(a), "Equal should be symmetric for %s", tt.name)
		})
	}
}
