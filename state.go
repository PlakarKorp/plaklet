package main

import "github.com/PlakarKorp/kloset/events"

// State mirrors the subset of plakman's reporting.State that plaklet produces:
// live progress counters, per-scope IO, and the CPU/RAM/throughput sample
// buffers. It is streamed as the raw JSON of a ReplyState. Keep the JSON tags in
// lockstep with plakman/reporting.State so the control plane can unmarshal it.
type State struct {
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

	// CPU/memory of this plaklet process (one process per job).
	Resources []ResourceSample `json:"resources,omitzero"`
	NumCPU    int              `json:"num_cpu,omitzero"`

	// Read/write throughput series, resolved per operation to kloset iostat
	// scopes.
	Network []NetworkSample `json:"network,omitzero"`

	Processed struct {
		Items uint64 `json:"items,omitzero"`
		Bytes uint64 `json:"bytes,omitzero"`
	} `json:"processed,omitzero"`

	// Latest per-scope I/O, keyed by kloset iostat scope name; feeds Network.
	IO map[string]IOScope `json:"io,omitzero"`
}

type StateCounter struct {
	Total      uint64 `json:"total,omitzero"`
	Ok         uint64 `json:"ok,omitzero"`
	Error      uint64 `json:"error,omitzero"`
	Size       uint64 `json:"size,omitzero"`
	Cached     uint64 `json:"cached,omitzero"`
	CachedSize uint64 `json:"cached_size,omitzero"`
}

// IODir is one direction of a kloset iostat scope. Overall is active-time
// throughput; OverallWall is wall-clock throughput (bytes/sec).
type IODir struct {
	TotalBytes  int64   `json:"total,omitzero"`
	Overall     float64 `json:"overall,omitzero"`
	OverallWall float64 `json:"overall_wall,omitzero"`
}

type IOScope struct {
	Read  IODir `json:"r,omitzero"`
	Write IODir `json:"w,omitzero"`
}

// MaxResourceSamples / MaxNetworkSamples cap the sample ring buffers; the
// samplers self-decimate when full so the buffer spans the whole run at a
// bounded size.
const (
	MaxResourceSamples = 120
	MaxNetworkSamples  = 120
)

// ResourceSample is one reading of this process's CPU%/RSS. Samples are not
// uniformly spaced on long runs; plot against At (unix millis).
type ResourceSample struct {
	At          int64   `json:"at"`
	CPUPercent  float64 `json:"cpu_percent"`
	MemoryBytes uint64  `json:"memory_bytes"`
}

// NetworkSample is one read/write throughput reading (bytes/sec). Read is
// wall-clock, write is active-time. Plot against At (unix millis).
type NetworkSample struct {
	At               int64   `json:"at"`
	ReadBytesPerSec  float64 `json:"read_bps,omitzero"`
	WriteBytesPerSec float64 `json:"write_bps,omitzero"`
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

// eventField extracts a typed value from an event's Data map, returning the zero
// value and false on a missing/mistyped key.
func eventField[T any](e events.Event, key string) (T, bool) {
	v, ok := e.Data[key].(T)
	return v, ok
}
