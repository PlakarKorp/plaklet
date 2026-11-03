package plaklet

import (
	"bytes"
	"errors"
	"fmt"
	"log"
	"testing"
	"time"

	"github.com/PlakarKorp/kloset/events"
	"github.com/PlakarKorp/kloset/objects"
	"github.com/stretchr/testify/require"
)

func event(typ string, data map[string]any) *events.Event {
	var id objects.MAC
	copy(id[:], []byte("snapshot-identifier-for-tests!!!"))
	return &events.Event{Type: typ, Level: "info", Snapshot: id, Workflow: "backup", Data: data}
}

// TestUpdateStateFoldsEveryEvent feeds one event of each kind the listener
// understands and checks the counters they land in.
func TestUpdateStateFoldsEveryEvent(t *testing.T) {
	fi := objects.NewFileInfo("f", 10, 0o644, time.Now(), 0, 0, 0, 0, 1)
	l := NewEventListener(true)

	for _, e := range []*events.Event{
		event("workflow.start", nil),
		event("workflow.end", nil),
		event("path", map[string]any{"path": "/a"}),
		event("path.ok", map[string]any{"path": "/a"}),
		event("path", map[string]any{"path": "/b"}),
		event("path.error", map[string]any{"path": "/b", "error": errors.New("boom")}),
		event("path", map[string]any{"path": "/c"}),
		event("path.cached", map[string]any{"path": "/c"}),
		event("directory", nil), event("directory.ok", nil), event("directory.error", nil), event("directory.cached", nil),
		event("file", nil), event("file.ok", map[string]any{"fileinfo": fi}), event("file.error", nil), event("file.cached", map[string]any{"fileinfo": fi}),
		event("xattr", nil), event("xattr.ok", map[string]any{"size": int64(3)}), event("xattr.error", nil), event("xattr.cached", map[string]any{"size": int64(4)}),
		event("symlink", nil), event("symlink.ok", nil), event("symlink.error", nil), event("symlink.cached", nil),
		event("object", nil), event("object.ok", nil), event("object.error", nil), event("object.cached", nil),
		event("chunk", nil), event("chunk.ok", nil), event("chunk.error", nil), event("chunk.cached", nil),
		event("fs.summary", map[string]any{"files": uint64(1), "directories": uint64(2), "symlinks": uint64(3), "xattrs": uint64(4), "size": uint64(5)}),
		event("snapshot.import.start", nil), event("snapshot.import.done", nil),
		event("snapshot.vfs.start", nil), event("snapshot.vfs.end", nil),
		event("snapshot.index.start", nil), event("snapshot.index.end", nil),
		event("snapshot.commit.start", nil),
		event("iostats", nil),
		event("iostats", map[string]any{"scope": ""}),
		event("iostats", map[string]any{
			"scope": "storage",
			"r":     map[string]any{"total": int64(7), "overall": 1.5},
			"w": map[string]any{"total": float64(9), "overall_wall": 2.5, "latency": map[string]any{
				"count": int64(2), "avg": time.Millisecond, "p50": 2 * time.Millisecond, "p95": 3 * time.Millisecond, "max": 4 * time.Millisecond,
			}},
		}),
		event("result", map[string]any{"size": uint64(11), "errors": uint64(1), "duration": 2 * time.Second}),
	} {
		l.handleEvent(e)
	}

	s := l.State()
	require.Equal(t, fmt.Sprintf("%x", event("", nil).Snapshot), s.SnapshotID)
	require.Equal(t, uint64(3), s.Paths.Total)
	require.Equal(t, uint64(1), s.Paths.Ok)
	require.Equal(t, uint64(1), s.Paths.Error)
	require.Equal(t, uint64(1), s.Paths.Cached)
	require.Len(t, s.RecentPaths, 3)
	for _, c := range []StateCounter{s.Dirs, s.Files, s.Xattrs, s.Symlinks, s.Objects, s.Chunks} {
		require.Equal(t, StateCounter{Total: 1, Ok: 1, Error: 1, Cached: 1, Size: c.Size, CachedSize: c.CachedSize}, c)
	}
	require.Equal(t, uint64(10), s.Files.Size)
	require.Equal(t, uint64(10), s.Files.CachedSize)
	require.Equal(t, uint64(3), s.Xattrs.Size)
	require.Equal(t, uint64(4), s.Xattrs.CachedSize)
	require.True(t, s.Summary.Exists)
	require.Equal(t, uint64(10), s.Summary.Paths)
	require.Equal(t, "completed", s.Phase)
	require.Equal(t, uint64(11), s.Result.Size)
	require.Equal(t, uint64(1), s.Result.Errors)
	require.Equal(t, 2.0, s.Result.Duration)
	require.Len(t, s.IO, 1)
	require.Equal(t, int64(7), s.IO["storage"].Read.TotalBytes)
	require.Equal(t, 1.5, s.IO["storage"].Read.Overall)
	require.Equal(t, int64(9), s.IO["storage"].Write.TotalBytes)
	require.Equal(t, 2.5, s.IO["storage"].Write.OverallWall)
	require.Equal(t, Latency{Count: 2, Avg: time.Millisecond, P50: 2 * time.Millisecond, P95: 3 * time.Millisecond, Max: 4 * time.Millisecond}, s.IO["storage"].Write.Latency)
	require.Equal(t, Latency{}, s.IO["storage"].Read.Latency)

	// A workflow without a snapshot leaves the id alone.
	var fresh State
	updateState(&fresh, events.Event{Type: "workflow.start"})
	require.Empty(t, fresh.SnapshotID)
}

// TestHandleEventLogs checks the lines the non-quiet listener writes for the
// events it reports, including the singular/plural of the result line.
func TestHandleEventLogs(t *testing.T) {
	var buf bytes.Buffer
	prev := log.Writer()
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(prev) })

	l := NewEventListener(false)
	var mac objects.MAC
	for _, e := range []*events.Event{
		event("path", map[string]any{"path": "/a"}),
		event("path.ok", map[string]any{"path": "/a"}),
		event("path.error", map[string]any{"path": "/b", "error": errors.New("boom")}),
		event("object", nil), event("object.ok", nil),
		event("object.error", map[string]any{"mac": mac, "error": errors.New("bad object")}),
		event("chunk.error", map[string]any{"mac": mac, "error": errors.New("bad chunk")}),
		event("result", map[string]any{"errors": uint64(1), "duration": time.Second, "rbytes": int64(1024), "wbytes": int64(2048)}),
		event("result", map[string]any{"errors": uint64(2), "duration": time.Second}),
		event("result", map[string]any{"errors": uint64(0), "duration": time.Second}),
	} {
		l.handleEvent(e)
	}
	out := buf.String()
	require.Contains(t, out, "OK /a")
	require.Contains(t, out, "KO /b: boom")
	require.Contains(t, out, "bad object")
	require.Contains(t, out, "bad chunk")
	require.Contains(t, out, "with 1 error in")
	require.Contains(t, out, "with 2 errors in")
	require.Contains(t, out, "without errors")

	// quiet drops the info-level lines.
	buf.Reset()
	q := NewEventListener(true)
	q.handleEvent(event("path.ok", map[string]any{"path": "/a"}))
	require.Empty(t, buf.String())
}

func TestRecentPathsAreCapped(t *testing.T) {
	var s State
	for i := range maxRecentPaths * 2 {
		startRecentPath(&s, fmt.Sprintf("/p%d", i))
	}
	require.Len(t, s.RecentPaths, maxRecentPaths)
	newest := fmt.Sprintf("/p%d", maxRecentPaths*2-1)
	require.Equal(t, RecentPath{Path: newest, Status: "in-progress"}, s.RecentPaths[0])
	settleRecentPath(&s, newest, "ok")
	require.Len(t, s.RecentPaths, maxRecentPaths)
	require.Equal(t, "in-progress", s.RecentPaths[0].Status)
	require.Equal(t, RecentPath{Path: newest, Status: "ok"}, s.RecentPaths[len(s.RecentPaths)-1])
}
