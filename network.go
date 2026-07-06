package main

import "time"

// networkSampler builds a read/write throughput series from the latest per-scope
// iostats. Which scopes count as read/write depends on the operation:
//
//	backup  : read = source,         write = storage
//	restore : read = storage,        write = destination
//	check   : read = storage,        write = (none)
//	sync    : read = source-storage, write = storage
//
// Read throughput is wall-clock (what the read path experiences); write is
// active-time (as exposed by kloset iostat). Self-decimates like resourceSampler.
type networkSampler struct {
	readScope  string
	writeScope string

	samples []NetworkSample
	stride  int
	tick    int
}

func newNetworkSampler(op string) *networkSampler {
	var read, write string
	switch op {
	case "backup":
		read, write = "source", "storage"
	case "restore":
		read, write = "storage", "destination"
	case "check":
		read, write = "storage", ""
	case "sync":
		read, write = "source-storage", "storage"
	}
	return &networkSampler{readScope: read, writeScope: write, stride: 1}
}

func (n *networkSampler) sample(io map[string]IOScope) {
	if n.readScope == "" && n.writeScope == "" {
		return
	}
	var readBps, writeBps float64
	if n.readScope != "" {
		readBps = io[n.readScope].Read.OverallWall
	}
	if n.writeScope != "" {
		writeBps = io[n.writeScope].Write.Overall
	}

	n.tick++
	if n.tick < n.stride {
		return
	}
	n.tick = 0

	n.record(NetworkSample{
		At:               time.Now().UnixMilli(),
		ReadBytesPerSec:  readBps,
		WriteBytesPerSec: writeBps,
	})
}

func (n *networkSampler) record(s NetworkSample) {
	n.samples = append(n.samples, s)
	if len(n.samples) > MaxNetworkSamples {
		kept := make([]NetworkSample, 0, len(n.samples)/2+1)
		for i := 0; i < len(n.samples); i += 2 {
			kept = append(kept, n.samples[i])
		}
		n.samples = kept
		n.stride *= 2
	}
}

func (n *networkSampler) snapshot() []NetworkSample {
	if len(n.samples) == 0 {
		return nil
	}
	out := make([]NetworkSample, len(n.samples))
	copy(out, n.samples)
	return out
}
