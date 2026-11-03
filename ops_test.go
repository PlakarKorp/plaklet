package plaklet

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/PlakarKorp/integration-sdk"
	"github.com/PlakarKorp/kloset/caching"
	"github.com/PlakarKorp/kloset/kcontext"
	"github.com/PlakarKorp/kloset/objects"
	"github.com/PlakarKorp/kloset/repository"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// The error paths of the operations, against real fs kloset repositories put
// in the state that trips each one.

var (
	noSuchIntegration = &Configuration{Integration: Integration{Name: "nope"}, Fields: []ConfigurationField{{Key: "location", Val: "nope://x"}}}
	srcID             = uuid.MustParse("11111111-1111-1111-1111-111111111111")
	storeID           = uuid.MustParse("22222222-2222-2222-2222-222222222222")
	destID            = uuid.MustParse("33333333-3333-3333-3333-333333333333")
	peerID            = uuid.MustParse("44444444-4444-4444-4444-444444444444")
)

// seededStore creates a store holding one backup of a one-file source, run on
// ctx, and returns the source and store configurations.
func seededStore(t *testing.T, ctx *kcontext.KContext) (source, store *Configuration) {
	t.Helper()
	srcDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "a.txt"), []byte("hello"), 0o644))
	source = fsConf(srcID, "importer", srcDir)
	store = fsConf(storeID, "storage", filepath.Join(t.TempDir(), "repo"))
	_, err := dispatch(ctx, &ExecPayload{Op: "create", Source: store}, nil)
	require.NoError(t, err)
	rep, err := dispatch(ctx, &ExecPayload{Op: "backup", Source: source, Target: store}, nil)
	require.NoError(t, err)
	require.NotNil(t, rep.Backup)
	return source, store
}

// dropPackfiles removes every packfile of a store: snapshot headers already in
// the context's repository cache still load, but nothing else can be read.
func dropPackfiles(t *testing.T, store *Configuration) {
	t.Helper()
	dir := store.params()["location"][len("fs://"):]
	require.NoError(t, os.RemoveAll(filepath.Join(dir, "packfiles")))
	require.NoError(t, os.Mkdir(filepath.Join(dir, "packfiles"), 0o700))
}

// lockExclusively leaves a fresh exclusive (maintenance) lock in the store.
func lockExclusively(t *testing.T, ctx *kcontext.KContext, store *Configuration) {
	t.Helper()
	st, pass, _, err := mkstorage(ctx, store)
	require.NoError(t, err)
	defer st.Close(ctx)
	repo, err := openrepo(ctx, st, pass)
	require.NoError(t, err)
	var buf bytes.Buffer
	require.NoError(t, repository.NewExclusiveLock("test").SerializeToStream(&buf))
	_, err = repo.PutLock(objects.RandomMAC(), &buf)
	require.NoError(t, err)
}

// cacheAsFile makes the named cache impossible to open by putting a regular
// file where its directory goes.
func cacheAsFile(t *testing.T, ctx *kcontext.KContext, name string) {
	t.Helper()
	dir := filepath.Join(ctx.CacheDir, caching.CACHE_VERSION)
	require.NoError(t, os.MkdirAll(dir, 0o700))
	require.NoError(t, os.RemoveAll(filepath.Join(dir, name)))
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), nil, 0o600))
}

func TestBackupErrors(t *testing.T) {
	ctx := newTestContext(t)
	source, store := seededStore(t, ctx)
	missing := fsConf(storeID, "storage", filepath.Join(t.TempDir(), "missing"))

	for name, payload := range map[string]*ExecPayload{
		"no source":       {Op: "backup", Target: store},
		"no target":       {Op: "backup", Source: source},
		"bad importer":    {Op: "backup", Source: fsConf(srcID, "importer", "relative"), Target: store},
		"bad store":       {Op: "backup", Source: source, Target: noSuchIntegration},
		"store not found": {Op: "backup", Source: source, Target: missing},
	} {
		_, err := dispatch(ctx, payload, nil)
		require.Error(t, err, name)
	}

	t.Run("locked store", func(t *testing.T) {
		ctx := newTestContext(t)
		_, store := seededStore(t, ctx)
		lockExclusively(t, ctx, store)
		_, err := dispatch(ctx, &ExecPayload{Op: "backup", Source: source, Target: store}, nil)
		require.Error(t, err)
	})

	t.Run("parent vfs unreadable", func(t *testing.T) {
		// The previous snapshot's header is cached, its data is gone: the
		// parent VFS cannot be built, and the backup goes on without it.
		ctx := newTestContext(t)
		source, store := seededStore(t, ctx)
		dropPackfiles(t, store)
		require.NoError(t, os.WriteFile(filepath.Join(source.params()["location"][len("fs://"):], "b.txt"), []byte("more"), 0o644))
		rep, err := dispatch(ctx, &ExecPayload{Op: "backup", Source: source, Target: store}, nil)
		require.NoError(t, err)
		require.NotNil(t, rep.Backup)
	})

	t.Run("import fails", func(t *testing.T) {
		ctx := newTestContext(t)
		_, store := seededStore(t, ctx)
		loadFakePlugin(t, "importer", integration.ConnectorTypeImporter, "")
		_, err := dispatch(ctx, &ExecPayload{Op: "backup", Source: fakeConf("importer", t.TempDir(), failField("import")), Target: store}, nil)
		require.Error(t, err)
	})

	t.Run("commit fails", func(t *testing.T) {
		ctx := newTestContext(t)
		source, store := seededStore(t, ctx)
		loadFakePlugin(t, "storage", integration.ConnectorTypeStorage, "")
		target := fakeConf("storage", store.params()["location"][len("fs://"):], failField("put:state"))
		_, err := dispatch(ctx, &ExecPayload{Op: "backup", Source: source, Target: target}, nil)
		require.Error(t, err)
	})
}

func TestCheckErrors(t *testing.T) {
	ctx := newTestContext(t)
	_, _ = seededStore(t, ctx)
	missing := fsConf(storeID, "storage", filepath.Join(t.TempDir(), "missing"))

	for name, payload := range map[string]*ExecPayload{
		"no source":       {Op: "check"},
		"bad store":       {Op: "check", Source: noSuchIntegration},
		"store not found": {Op: "check", Source: missing},
	} {
		_, err := dispatch(ctx, payload, nil)
		require.Error(t, err, name)
	}

	t.Run("check cache unavailable", func(t *testing.T) {
		ctx := newTestContext(t)
		_, store := seededStore(t, ctx)
		cacheAsFile(t, ctx, "check")
		_, err := dispatch(ctx, &ExecPayload{Op: "check", Source: store}, nil)
		require.Error(t, err)
	})

	t.Run("corrupted snapshot", func(t *testing.T) {
		ctx := newTestContext(t)
		_, store := seededStore(t, ctx)
		dropPackfiles(t, store)
		rep, err := dispatch(ctx, &ExecPayload{Op: "check", Source: store}, nil)
		require.Error(t, err)
		require.NotNil(t, rep)
		require.Equal(t, uint64(1), rep.Check.Errors)
	})
}

func TestCreateErrors(t *testing.T) {
	ctx := newTestContext(t)

	_, err := dispatch(ctx, &ExecPayload{Op: "create"}, nil)
	require.Error(t, err)
	_, err = dispatch(ctx, &ExecPayload{Op: "create", Source: noSuchIntegration}, nil)
	require.Error(t, err)

	// A store cannot be created where one already is.
	_, store := seededStore(t, ctx)
	_, err = dispatch(ctx, &ExecPayload{Op: "create", Source: store}, nil)
	require.Error(t, err)

	// Compression can be turned off.
	_, err = dispatch(ctx, &ExecPayload{
		Op:         "create",
		TaskConfig: map[string]string{"no_compression": "true"},
		Source:     fsConf(storeID, "storage", filepath.Join(t.TempDir(), "repo")),
	}, nil)
	require.NoError(t, err)
}

func TestRestoreErrors(t *testing.T) {
	ctx := newTestContext(t)
	source, store := seededStore(t, ctx)
	dest := fsConf(destID, "exporter", t.TempDir())
	missing := fsConf(storeID, "storage", filepath.Join(t.TempDir(), "missing"))
	empty := fsConf(storeID, "storage", filepath.Join(t.TempDir(), "empty"))
	createFSRepo(t, ctx, empty.params()["location"][len("fs://"):])

	// A second snapshot, so an id filter matching every snapshot is ambiguous.
	_, err := dispatch(ctx, &ExecPayload{Op: "backup", Source: source, Target: store}, nil)
	require.NoError(t, err)

	for name, payload := range map[string]*ExecPayload{
		"no source":       {Op: "restore", Target: dest},
		"no target":       {Op: "restore", Source: store},
		"bad exporter":    {Op: "restore", Source: store, Target: fsConf(destID, "exporter", t.TempDir(), ConfigurationField{Key: "skip_ownership", Val: "maybe"})},
		"bad store":       {Op: "restore", Source: noSuchIntegration, Target: dest},
		"store not found": {Op: "restore", Source: missing, Target: dest},
		"no snapshot":     {Op: "restore", Source: empty, Target: dest},
		"ambiguous":       {Op: "restore", TaskConfig: map[string]string{"locate.filter.ids.0": ""}, Source: store, Target: dest},
	} {
		_, err := dispatch(ctx, payload, nil)
		require.Error(t, err, name)
	}

	t.Run("corrupted snapshot", func(t *testing.T) {
		ctx := newTestContext(t)
		_, store := seededStore(t, ctx)
		dropPackfiles(t, store)
		_, err := dispatch(ctx, &ExecPayload{Op: "restore", Source: store, Target: dest}, nil)
		require.Error(t, err)
	})
}

func TestSyncErrors(t *testing.T) {
	ctx := newTestContext(t)
	_, store := seededStore(t, ctx)
	peer := fsConf(peerID, "storage", filepath.Join(t.TempDir(), "peer"))
	createFSRepo(t, ctx, peer.params()["location"][len("fs://"):])
	missing := fsConf(storeID, "storage", filepath.Join(t.TempDir(), "missing"))

	for name, payload := range map[string]*ExecPayload{
		"no source":      {Op: "sync", Target: peer},
		"no target":      {Op: "sync", Source: store},
		"bad source":     {Op: "sync", Source: noSuchIntegration, Target: peer},
		"source missing": {Op: "sync", Source: missing, Target: peer},
		"bad target":     {Op: "sync", Source: store, Target: noSuchIntegration},
		"target missing": {Op: "sync", Source: store, Target: missing},
	} {
		_, err := dispatch(ctx, payload, nil)
		require.Error(t, err, name)
	}

	t.Run("locked peer", func(t *testing.T) {
		lockExclusively(t, ctx, peer)
		rep, err := dispatch(ctx, &ExecPayload{Op: "sync", Source: store, Target: peer}, nil)
		require.NoError(t, err)
		require.Equal(t, uint64(1), rep.Sync.Errors)
	})

	t.Run("corrupted source", func(t *testing.T) {
		ctx := newTestContext(t)
		_, store := seededStore(t, ctx)
		peer := fsConf(peerID, "storage", filepath.Join(t.TempDir(), "peer"))
		createFSRepo(t, ctx, peer.params()["location"][len("fs://"):])
		dropPackfiles(t, store)
		rep, err := dispatch(ctx, &ExecPayload{Op: "sync", Source: store, Target: peer}, nil)
		require.NoError(t, err)
		require.Equal(t, uint64(1), rep.Sync.Errors)
	})

	t.Run("parent vfs unreadable", func(t *testing.T) {
		// A first sync seeds the peer; its data then goes away while its
		// header stays cached, so the second sync finds a parent it cannot
		// build the VFS of.
		ctx := newTestContext(t)
		source, store := seededStore(t, ctx)
		peer := fsConf(peerID, "storage", filepath.Join(t.TempDir(), "peer"))
		createFSRepo(t, ctx, peer.params()["location"][len("fs://"):])
		rep, err := dispatch(ctx, &ExecPayload{Op: "sync", Source: store, Target: peer}, nil)
		require.NoError(t, err)
		require.Len(t, rep.Sync.Syncs, 1)

		require.NoError(t, os.WriteFile(filepath.Join(source.params()["location"][len("fs://"):], "b.txt"), []byte("more"), 0o644))
		_, err = dispatch(ctx, &ExecPayload{Op: "backup", Source: source, Target: store}, nil)
		require.NoError(t, err)
		dropPackfiles(t, peer)
		rep, err = dispatch(ctx, &ExecPayload{Op: "sync", Source: store, Target: peer}, nil)
		require.NoError(t, err)
		require.Equal(t, uint64(1), rep.Sync.Errors)
	})
}

// TestSyncReusesParent is the healthy counterpart: the second sync of a
// source seeds its VFS cache from the peer's first copy.
func TestSyncReusesParent(t *testing.T) {
	ctx := newTestContext(t)
	source, store := seededStore(t, ctx)
	peer := fsConf(peerID, "storage", filepath.Join(t.TempDir(), "peer"))
	createFSRepo(t, ctx, peer.params()["location"][len("fs://"):])
	_, err := dispatch(ctx, &ExecPayload{Op: "sync", Source: store, Target: peer}, nil)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(source.params()["location"][len("fs://"):], "b.txt"), []byte("more"), 0o644))
	_, err = dispatch(ctx, &ExecPayload{Op: "backup", Source: source, Target: store}, nil)
	require.NoError(t, err)
	rep, err := dispatch(ctx, &ExecPayload{Op: "sync", Source: store, Target: peer}, nil)
	require.NoError(t, err)
	require.Len(t, rep.Sync.Syncs, 1)
	require.Zero(t, rep.Sync.Errors)
}

func TestRmErrors(t *testing.T) {
	ctx := newTestContext(t)
	_, store := seededStore(t, ctx)
	ids := map[string]string{"snapshot_ids.0": "00000000000000000000000000000000ab000000000000000000000000000000"}

	_, err := dispatch(ctx, &ExecPayload{Op: "rm"}, nil)
	require.Error(t, err)
	_, err = dispatch(ctx, &ExecPayload{Op: "rm", TaskConfig: map[string]string{"snapshot_ids.0": "zz"}, Source: store}, nil)
	require.Error(t, err)
	_, err = dispatch(ctx, &ExecPayload{Op: "rm", TaskConfig: ids, Source: noSuchIntegration}, nil)
	require.Error(t, err)
	_, err = dispatch(ctx, &ExecPayload{Op: "rm", TaskConfig: ids, Source: fsConf(storeID, "storage", filepath.Join(t.TempDir(), "missing"))}, nil)
	require.Error(t, err)

	// A removal that fails is counted in the report, not returned.
	t.Run("delete fails", func(t *testing.T) {
		ctx := newTestContext(t)
		_, store := seededStore(t, ctx)
		cacheAsFile(t, ctx, "scan")
		rep, err := dispatch(ctx, &ExecPayload{Op: "rm", TaskConfig: ids, Source: store}, nil)
		require.NoError(t, err)
		require.Equal(t, uint64(1), rep.Rm.Errors)
		require.Empty(t, rep.Rm.SnapshotIDs)
	})
}

func TestUnsupportedOps(t *testing.T) {
	ctx := newTestContext(t)
	for _, op := range []string{"prune", "maintenance"} {
		_, err := dispatch(ctx, &ExecPayload{Op: op}, nil)
		require.Error(t, err, op)
		_, err = dispatch(ctx, &ExecPayload{Op: op, Source: fsConf(storeID, "storage", t.TempDir())}, nil)
		require.ErrorIs(t, err, errors.ErrUnsupported, op)
	}
}

func TestTestOp(t *testing.T) {
	ctx := newTestContext(t)
	_, store := seededStore(t, ctx)
	srcDir := t.TempDir()

	_, err := dispatch(ctx, &ExecPayload{Op: "test"}, nil)
	require.Error(t, err)
	_, err = dispatch(ctx, &ExecPayload{Op: "test", Source: &Configuration{Type: "bogus"}}, nil)
	require.Error(t, err)

	conf := func(typ string, c *Configuration) *Configuration {
		cc := *c
		cc.Type = typ
		return &cc
	}

	// store
	_, err = dispatch(ctx, &ExecPayload{Op: "test", Source: conf("store", store)}, nil)
	require.NoError(t, err)
	_, err = dispatch(ctx, &ExecPayload{Op: "test", Source: conf("store", noSuchIntegration)}, nil)
	require.Error(t, err)
	_, err = dispatch(ctx, &ExecPayload{Op: "test", Source: conf("store", fsConf(storeID, "storage", filepath.Join(t.TempDir(), "missing")))}, nil)
	require.Error(t, err)

	// source
	_, err = dispatch(ctx, &ExecPayload{Op: "test", Source: conf("source", fsConf(srcID, "importer", srcDir))}, nil)
	require.NoError(t, err)
	_, err = dispatch(ctx, &ExecPayload{Op: "test", Source: conf("source", noSuchIntegration)}, nil)
	require.Error(t, err)

	// destination
	_, err = dispatch(ctx, &ExecPayload{Op: "test", Source: conf("destination", fsConf(destID, "exporter", srcDir))}, nil)
	require.NoError(t, err)
	_, err = dispatch(ctx, &ExecPayload{Op: "test", Source: conf("destination", noSuchIntegration)}, nil)
	require.Error(t, err)
}

func TestInventoryThroughPlugin(t *testing.T) {
	ctx := newTestContext(t)
	loadFakePlugin(t, "inventory", integration.ConnectorTypeInventory, "")

	var replies []*ExecReply
	send := func(r *ExecReply) { replies = append(replies, r) }
	_, err := dispatch(ctx, &ExecPayload{Op: "inventory", Source: fakeConf("inventory", "")}, send)
	require.NoError(t, err)
	require.Len(t, replies, 2)
	for _, r := range replies {
		require.Equal(t, ReplyInventory, r.Type)
	}

	// A plugin that fails to start.
	_, err = dispatch(ctx, &ExecPayload{Op: "inventory", Source: fakeConf("inventory", "", failField("init"))}, send)
	require.Error(t, err)
}
