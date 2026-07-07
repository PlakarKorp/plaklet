package plaklet

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/PlakarKorp/kloset/caching"
	"github.com/PlakarKorp/kloset/caching/pebble"
	"github.com/PlakarKorp/kloset/connectors/storage"
	"github.com/PlakarKorp/kloset/hashing"
	"github.com/PlakarKorp/kloset/kcontext"
	"github.com/PlakarKorp/kloset/logging"
	"github.com/PlakarKorp/kloset/resources"
	"github.com/PlakarKorp/kloset/versioning"
	"github.com/stretchr/testify/require"
)

// newTestContext builds a kcontext with a real pebble cache in a temp dir and a
// sane concurrency, matching what main() sets up.
func newTestContext(t *testing.T) *kcontext.KContext {
	t.Helper()
	cachedir := t.TempDir()
	ctx := kcontext.NewKContext()
	ctx.CacheDir = cachedir
	ctx.MaxConcurrency = 4
	ctx.SetLogger(logging.NewLogger(io.Discard, io.Discard))
	ctx.SetCache(caching.NewManager(pebble.Constructor(cachedir)))

	// kloset blocks publishing progress once the event buffer fills, so the bus
	// must be drained for the duration of any op (main() does this via the
	// listener). Drain and discard here.
	bus := ctx.Events()
	drained := make(chan struct{})
	go func() {
		for range bus.Listen() {
		}
		close(drained)
	}()
	t.Cleanup(func() {
		bus.Close()
		<-drained
		ctx.GetCache().Close()
	})
	return ctx
}

// createFSRepo initializes an unencrypted kloset store at dir.
func createFSRepo(t *testing.T, ctx *kcontext.KContext, dir string) {
	t.Helper()
	st, err := storage.New(ctx, map[string]string{"location": "fs://" + dir})
	require.NoError(t, err)

	config := storage.NewConfiguration()
	config.Encryption = nil
	serialized, err := config.ToBytes()
	require.NoError(t, err)

	hasher := hashing.GetHasher(hashing.DEFAULT_HASHING_ALGORITHM)
	wrappedRd, err := storage.Serialize(hasher, resources.RT_CONFIG, versioning.GetCurrentVersion(resources.RT_CONFIG), bytes.NewReader(serialized))
	require.NoError(t, err)
	wrapped, err := io.ReadAll(wrappedRd)
	require.NoError(t, err)

	require.NoError(t, st.Create(ctx, wrapped))
	require.NoError(t, st.Close(ctx))
}

func fsConf(id, typ, location string) *Configuration {
	return &Configuration{
		Id:          id,
		Type:        typ,
		Integration: Integration{Name: "fs", Version: "test"},
		Name:        typ,
		Fields:      []ConfigurationField{{Key: "location", Val: "fs://" + location}},
	}
}

// TestEndToEnd drives the full operation set against real fs kloset repos:
// backup a source, check it, restore it (byte-identical), then sync to a peer.
func TestEndToEnd(t *testing.T) {
	srcDir := t.TempDir()
	repoDir := filepath.Join(t.TempDir(), "repo")
	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "a.txt"), []byte("hello"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "b.txt"), []byte("world!!"), 0o644))

	source := fsConf("11111111-1111-1111-1111-111111111111", "importer", srcDir)
	store := fsConf("22222222-2222-2222-2222-222222222222", "storage", repoDir)
	restoreDir := t.TempDir()
	peerDir := filepath.Join(t.TempDir(), "peer")
	peer := fsConf("44444444-4444-4444-4444-444444444444", "storage", peerDir)

	// Each phase runs as a subtest so its context (and pebble cache) is torn down
	// via t.Cleanup before the next phase begins, rather than all contexts piling
	// up until the parent test ends.
	t.Run("create", func(t *testing.T) {
		ctx := newTestContext(t)
		// Create the store via the create op (unencrypted: no passphrase field).
		_, err := dispatch(ctx, &ExecPayload{
			Op:         "create",
			TaskConfig: map[string]string{"no_encryption": "true"},
			Source:     store,
		})
		require.NoError(t, err)
	})

	t.Run("backup", func(t *testing.T) {
		ctx := newTestContext(t)
		rep, err := dispatch(ctx, &ExecPayload{
			Op:         "backup",
			TaskConfig: map[string]string{"tags": "e2e"},
			Source:     source,
			Target:     store,
		})
		require.NoError(t, err)
		require.NotNil(t, rep.Backup)
		require.Equal(t, uint64(2), rep.Backup.Content.Files)
		require.NotEmpty(t, rep.Backup.SnapshotID)
		require.Contains(t, rep.Backup.Tags, "e2e")
	})

	t.Run("check", func(t *testing.T) {
		ctx := newTestContext(t)
		rep, err := dispatch(ctx, &ExecPayload{Op: "check", Source: store})
		require.NoError(t, err)
		require.NotNil(t, rep.Check)
		require.Len(t, rep.Check.Checks, 1)
		require.Zero(t, rep.Check.Errors)
	})

	t.Run("restore", func(t *testing.T) {
		ctx := newTestContext(t)
		dest := fsConf("33333333-3333-3333-3333-333333333333", "exporter", restoreDir)
		rep, err := dispatch(ctx, &ExecPayload{
			Op:         "restore",
			TaskConfig: map[string]string{"latest": "true"},
			Source:     store,
			Target:     dest,
		})
		require.NoError(t, err)
		require.NotNil(t, rep.Restore)

		got, err := os.ReadFile(filepath.Join(restoreDir, "a.txt"))
		require.NoError(t, err)
		require.Equal(t, "hello", string(got))
		got, err = os.ReadFile(filepath.Join(restoreDir, "b.txt"))
		require.NoError(t, err)
		require.Equal(t, "world!!", string(got))
	})

	t.Run("sync", func(t *testing.T) {
		ctx := newTestContext(t)
		createFSRepo(t, ctx, peerDir)
		rep, err := dispatch(ctx, &ExecPayload{Op: "sync", Source: store, Target: peer})
		require.NoError(t, err)
		require.NotNil(t, rep.Sync)
		require.Len(t, rep.Sync.Syncs, 1, "one snapshot should copy")
		require.Zero(t, rep.Sync.Errors)
	})

	t.Run("sync-idempotent", func(t *testing.T) {
		ctx := newTestContext(t)
		rep, err := dispatch(ctx, &ExecPayload{Op: "sync", Source: store, Target: peer})
		require.NoError(t, err)
		require.NotNil(t, rep.Sync)
		require.Empty(t, rep.Sync.Syncs, "second sync copies nothing")
	})
}

func TestDispatchUnsupportedOp(t *testing.T) {
	// A bare context is enough: dispatch rejects the op before touching kloset.
	_, err := dispatch(kcontext.NewKContext(), &ExecPayload{Op: "bogus"})
	require.Error(t, err)
}

// TestIncrementalBackupDedups proves the StateRefresher works: backing up an
// unchanged source twice must dedup against the first snapshot's chunks, which
// only happens if the first backup's state was folded into the repository's
// aggregated state (what plakman's cached daemon does; the edge does it in
// process). Without the refresher the second backup sees no prior chunks and
// caches nothing. Uses its own store so it doesn't perturb TestEndToEnd.
func TestIncrementalBackupDedups(t *testing.T) {
	srcDir := t.TempDir()
	repoDir := filepath.Join(t.TempDir(), "repo")
	// A file large enough to be a real chunk, so dedup is meaningful.
	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "data.bin"),
		bytes.Repeat([]byte("plaklet-incremental-"), 4096), 0o644))

	source := fsConf("11111111-1111-1111-1111-111111111111", "importer", srcDir)
	store := fsConf("22222222-2222-2222-2222-222222222222", "storage", repoDir)

	run := func(op string, src, tgt *Configuration) *Report {
		ctx := newTestContext(t)
		rep, err := dispatch(ctx, &ExecPayload{Op: op, Source: src, Target: tgt})
		require.NoError(t, err)
		return rep
	}

	run("create", store, nil)
	first := run("backup", source, store)
	require.NotNil(t, first.Backup)

	second := run("backup", source, store)
	require.NotNil(t, second.Backup)

	// The second backup must have cached (deduped) chunks — the repo saw the
	// first snapshot's state. Chunk counts live in the state stream, but the
	// report's store bytes-written is the observable proxy: the second run writes
	// strictly less than the first (only the new snapshot's metadata, not the
	// re-chunked file).
	require.Less(t, second.Backup.Store.BytesWritten, first.Backup.Store.BytesWritten,
		"second backup wrote >= first; aggregated state was not refreshed (dedup failed)")

	// And the repository is consistent: check both snapshots, no errors.
	chk := run("check", store, nil)
	require.NotNil(t, chk.Check)
	require.Len(t, chk.Check.Checks, 2)
	require.Zero(t, chk.Check.Errors)
}
