package main

import (
	"sync"
	"time"

	"github.com/PlakarKorp/kloset/events"
	"github.com/PlakarKorp/kloset/objects"
)

// eventListener consumes the kloset event bus and folds events into a live
// State: progress counters, phase, per-scope IO. A background sampler reads
// State() on a ticker to build the streamed state (see main.go). Draining the
// bus is also what keeps kloset's importer from blocking on a full event
// channel.
type eventListener struct {
	done chan struct{}

	mu    sync.Mutex
	state State
}

func newEventListener() *eventListener {
	return &eventListener{}
}

// Run starts draining the bus in a goroutine. It returns immediately; Wait
// blocks until the bus is closed.
func (l *eventListener) Run(bus *events.EventsBUS) {
	l.done = make(chan struct{})
	go func() {
		defer close(l.done)
		for e := range bus.Listen() {
			l.mu.Lock()
			updateState(&l.state, *e)
			l.mu.Unlock()
		}
	}()
}

func (l *eventListener) Wait() { <-l.done }

// State returns a copy of the current state with the running processed counters
// filled in.
func (l *eventListener) State() State {
	l.mu.Lock()
	defer l.mu.Unlock()
	s := l.state
	s.Processed.Items, s.Processed.Bytes = s.processed()
	// IO is a map (reference); copy it so the caller can send it without racing
	// further mutations.
	if l.state.IO != nil {
		s.IO = make(map[string]IOScope, len(l.state.IO))
		for k, v := range l.state.IO {
			s.IO[k] = v
		}
	}
	return s
}

func updateState(s *State, e events.Event) {
	switch e.Type {
	case "workflow.start":
		if e.Snapshot != objects.NilMac {
			s.SnapshotID = e.Snapshot.FormatHex()
		}

	case "path":
		s.Paths.Total++
	case "path.ok":
		s.Paths.Ok++
	case "path.error":
		s.Paths.Error++
	case "path.cached":
		s.Paths.Cached++

	case "directory":
		s.Dirs.Total++
	case "directory.ok":
		s.Dirs.Ok++
	case "directory.error":
		s.Dirs.Error++
	case "directory.cached":
		s.Dirs.Cached++

	case "file":
		s.Files.Total++
	case "file.ok":
		s.Files.Ok++
		fi, _ := eventField[objects.FileInfo](e, "fileinfo")
		s.Files.Size += uint64(fi.Size())
	case "file.error":
		s.Files.Error++
	case "file.cached":
		s.Files.Cached++
		fi, _ := eventField[objects.FileInfo](e, "fileinfo")
		s.Files.CachedSize += uint64(fi.Size())

	case "xattr":
		s.Xattrs.Total++
	case "xattr.ok":
		s.Xattrs.Ok++
		size, _ := eventField[int64](e, "size")
		s.Xattrs.Size += uint64(size)
	case "xattr.error":
		s.Xattrs.Error++
	case "xattr.cached":
		s.Xattrs.Cached++
		size, _ := eventField[int64](e, "size")
		s.Xattrs.CachedSize += uint64(size)

	case "symlink":
		s.Symlinks.Total++
	case "symlink.ok":
		s.Symlinks.Ok++
	case "symlink.error":
		s.Symlinks.Error++
	case "symlink.cached":
		s.Symlinks.Cached++

	case "object":
		s.Objects.Total++
	case "object.ok":
		s.Objects.Ok++
	case "object.error":
		s.Objects.Error++
	case "object.cached":
		s.Objects.Cached++

	case "chunk":
		s.Chunks.Total++
	case "chunk.ok":
		s.Chunks.Ok++
	case "chunk.error":
		s.Chunks.Error++
	case "chunk.cached":
		s.Chunks.Cached++

	case "fs.summary":
		s.Summary.Exists = true
		s.Summary.Files, _ = eventField[uint64](e, "files")
		s.Summary.Directories, _ = eventField[uint64](e, "directories")
		s.Summary.Symlinks, _ = eventField[uint64](e, "symlinks")
		s.Summary.Xattrs, _ = eventField[uint64](e, "xattrs")
		s.Summary.Size, _ = eventField[uint64](e, "size")
		s.Summary.Paths = s.Summary.Files + s.Summary.Directories + s.Summary.Symlinks + s.Summary.Xattrs

	case "snapshot.import.start":
		s.Phase = "processing"
	case "snapshot.vfs.start":
		s.Phase = "building VFS"
	case "snapshot.vfs.end":
		s.Phase = ""
	case "snapshot.index.start":
		s.Phase = "indexing"
	case "snapshot.index.end":
		s.Phase = ""
	case "snapshot.commit.start":
		s.Phase = "committing"

	case "iostats":
		scope, ok := eventField[string](e, "scope")
		if !ok || scope == "" {
			return
		}
		if s.IO == nil {
			s.IO = make(map[string]IOScope)
		}
		s.IO[scope] = IOScope{
			Read:  ioDirFromEvent(e, "r"),
			Write: ioDirFromEvent(e, "w"),
		}

	case "result":
		s.Phase = "completed"
		s.Result.Size, _ = eventField[uint64](e, "size")
		s.Result.Errors, _ = eventField[uint64](e, "errors")
		if d, ok := eventField[time.Duration](e, "duration"); ok {
			s.Result.Duration = d.Seconds()
		}
	}
}

// ioDirFromEvent pulls one direction ("r"/"w") of an iostats event into an
// IODir; missing/mistyped fields default to zero.
func ioDirFromEvent(e events.Event, key string) IODir {
	dir, _ := eventField[map[string]any](e, key)
	getf := func(k string) float64 {
		switch n := dir[k].(type) {
		case float64:
			return n
		case int64:
			return float64(n)
		default:
			return 0
		}
	}
	return IODir{
		TotalBytes:  int64(getf("total")),
		Overall:     getf("overall"),
		OverallWall: getf("overall_wall"),
	}
}
