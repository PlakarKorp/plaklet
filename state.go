package plaklet

import (
	"maps"
	"slices"
	"time"
)

type State struct {
	Version    string `json:"version,omitzero"`
	SnapshotID string `json:"snapshot_id,omitzero"`
	Phase      string `json:"phase,omitzero"`

	Summary struct {
		Exists      bool   `json:"exists,omitzero"`
		Paths       uint64 `json:"paths,omitzero"`
		Files       uint64 `json:"files,omitzero"`
		Directories uint64 `json:"directories,omitzero"`
		Symlinks    uint64 `json:"symlinks,omitzero"`
		Xattrs      uint64 `json:"xattrs,omitzero"`
		Size        uint64 `json:"size,omitzero"`
	} `json:"summary,omitzero"`

	Paths    StateCounter `json:"paths,omitzero"`
	Dirs     StateCounter `json:"dirs,omitzero"`
	Files    StateCounter `json:"files,omitzero"`
	Xattrs   StateCounter `json:"xattrs,omitzero"`
	Symlinks StateCounter `json:"symlinks,omitzero"`
	Chunks   StateCounter `json:"chunks,omitzero"`
	Objects  StateCounter `json:"objects,omitzero"`

	Result struct {
		Size     uint64  `json:"size,omitzero"`
		Errors   uint64  `json:"errors,omitzero"`
		Duration float64 `json:"duration,omitzero"`
	} `json:"result,omitzero"`

	// Rolling window of recent process resource samples (CPU/memory of the
	// plaklet process, which is one-per-job). A ring buffer capped at
	// MaxResourceSamples so the UI can draw a live graph even if it opens mid
	// run or misses polls. Not persisted beyond the live state.
	Resources []ResourceSample `json:"resources,omitzero"`

	// Number of CPUs available to the process, so the UI can read the
	// whole-process CPUPercent against its ceiling (e.g. "380% / 8 cores").
	NumCPU int `json:"num_cpu,omitzero"`

	// Rolling window of network throughput samples for the read and write
	// paths, resolved per operation to the relevant kloset iostat scopes
	// (backup: read=source/write=storage; restore: read=storage/write=
	// destination; check: read=storage; sync: read=source/write=target-store).
	// Read uses wall-clock throughput, write uses active-time throughput. Same
	// self-decimating ring buffer as Resources; not persisted beyond live state.
	Network []NetworkSample `json:"network,omitzero"`

	// Rolling window of write-latency samples for the job's storage scope.
	// Same self-decimating ring buffer as Network.
	Latency []LatencySample `json:"latency,omitzero"`

	// Bytes/items processed so far. The total is unknown up front, so this is a
	// running count for a progress readout, not a percentage.
	Processed struct {
		Items uint64 `json:"items,omitzero"`
		Bytes uint64 `json:"bytes,omitzero"`
	} `json:"processed,omitzero"`

	// Latest per-scope I/O stats, keyed by kloset iostat scope name. Updated
	// from "iostats" events; feeds the network throughput sampling.
	IO map[string]IOScope `json:"io,omitzero"`

	RecentPaths []RecentPath `json:"recent_paths,omitzero"`
}

type StateCounter struct {
	Total      uint64 `json:"total,omitzero"`
	Ok         uint64 `json:"ok,omitzero"`
	Error      uint64 `json:"error,omitzero"`
	Size       uint64 `json:"size,omitzero"`
	Cached     uint64 `json:"cached,omitzero"`
	CachedSize uint64 `json:"cached_size,omitzero"`
}

// IODir is one direction (read or write) of a kloset iostat scope. Overall is
// active-time throughput; OverallWall is wall-clock throughput (bytes/sec).
type IODir struct {
	TotalBytes  int64   `json:"total,omitzero"`
	Overall     float64 `json:"overall,omitzero"`
	OverallWall float64 `json:"overall_wall,omitzero"`

	Latency Latency `json:"latency,omitzero"`
}

// Latency is the distribution of individual operation durations in one
// direction of a scope. The percentiles come from a geometric histogram and
// are unreliable below a few dozen operations, so read them against Count.
//
// kloset observes latency on packfile writes to the storage backend only, and
// not on the ones that fail. Other scopes stay zero.
type Latency struct {
	Count int64         `json:"count,omitzero"`
	Avg   time.Duration `json:"avg,omitzero"`
	P50   time.Duration `json:"p50,omitzero"`
	P95   time.Duration `json:"p95,omitzero"`
	Max   time.Duration `json:"max,omitzero"`
}

// IOScope holds the latest read/write stats of one kloset iostat scope.
type IOScope struct {
	Read  IODir `json:"r,omitzero"`
	Write IODir `json:"w,omitzero"`
}

// MaxResourceSamples caps the resource buffer. The sampler self-decimates when
// full (halving resolution, doubling the interval) so this many points always
// span the whole run rather than a fixed time window: a short job is sampled
// every second, an hours-long one every few minutes. The state stays a few KB
// regardless of duration.

// MaxNetworkSamples caps the network throughput buffer; same self-decimating
// scheme as MaxResourceSamples so it spans the whole run at a bounded size.

// MaxLatencySamples caps the latency buffer; same scheme as MaxNetworkSamples.
const (
	MaxResourceSamples = 120
	MaxNetworkSamples  = 120
	MaxLatencySamples  = 120
)

// ResourceSample is one point-in-time reading of the plaklet process's resource
// usage. Samples are NOT uniformly spaced (the sampler decimates older history
// on long runs), so the UI must place them on the time axis using At, not by
// index.
type ResourceSample struct {
	At          int64   `json:"at"`           // unix millis
	CPUPercent  float64 `json:"cpu_percent"`  // whole-process CPU %, may exceed 100 on multi-core
	MemoryBytes uint64  `json:"memory_bytes"` // resident set size
}

// NetworkSample is one point-in-time reading of read/write throughput (bytes
// per second). ReadBytesPerSec is wall-clock throughput; WriteBytesPerSec is
// active-time throughput. Samples are NOT uniformly spaced (decimated on long
// runs); place them on the time axis using At.
type NetworkSample struct {
	At               int64   `json:"at"`                 // unix millis
	ReadBytesPerSec  float64 `json:"read_bps,omitzero"`  // wall-clock
	WriteBytesPerSec float64 `json:"write_bps,omitzero"` // active-time
}

// maxRecentPaths caps RecentPaths: in-progress paths first, then the most
// recently settled ones.
const maxRecentPaths = 5

type RecentPath struct {
	Path   string `json:"path"`
	Status string `json:"status"`
}

// LatencySample is one reading of the write-path latency distribution, taken
// cumulatively over the run to that instant rather than over the interval
// since the previous point. Not uniformly spaced: place them using At.
//
// Count is the observation count behind the percentiles. It only ever grows,
// so a series that stalls at a fixed Count means writes stopped completing —
// kloset does not observe the ones that fail.
type LatencySample struct {
	At    int64         `json:"at"` // unix millis
	Count int64         `json:"count,omitzero"`
	Avg   time.Duration `json:"avg,omitzero"`
	P50   time.Duration `json:"p50,omitzero"`
	P95   time.Duration `json:"p95,omitzero"`
	Max   time.Duration `json:"max,omitzero"`
}

// processed fills the running items/bytes readout from the per-file counters,
// falling back to the scan summary (check/export emit fs.summary; backup does
// not).
func (s *State) processed() (items, bytes uint64) {
	items = s.Files.Ok + s.Files.Cached
	bytes = s.Files.Size + s.Files.CachedSize
	if bytes == 0 {
		bytes = s.Summary.Size
	}
	if items == 0 {
		items = s.Summary.Files
	}
	return
}

func (s State) Equal(other State) bool {
	return s.Version == other.Version &&
		s.SnapshotID == other.SnapshotID &&
		s.Phase == other.Phase &&
		s.Summary == other.Summary &&
		s.Paths == other.Paths &&
		s.Dirs == other.Dirs &&
		s.Files == other.Files &&
		s.Xattrs == other.Xattrs &&
		s.Symlinks == other.Symlinks &&
		s.Chunks == other.Chunks &&
		s.Objects == other.Objects &&
		s.Result == other.Result &&
		slices.Equal(s.Resources, other.Resources) &&
		s.NumCPU == other.NumCPU &&
		slices.Equal(s.Network, other.Network) &&
		slices.Equal(s.Latency, other.Latency) &&
		s.Processed == other.Processed &&
		maps.Equal(s.IO, other.IO) &&
		slices.Equal(s.RecentPaths, other.RecentPaths)
}
