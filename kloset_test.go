package plaklet

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/PlakarKorp/integration-sdk"
	_ "github.com/PlakarKorp/integrations/fs/importer"
	_ "github.com/PlakarKorp/integrations/tar/importer"
	"github.com/PlakarKorp/kloset/connectors/storage"
	"github.com/PlakarKorp/kloset/encryption"
	"github.com/PlakarKorp/plaklet/plugin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// captureLog routes the package log sink to a slice for the test's duration
// and returns it; the sink is otherwise unset outside Main.
func captureLog(t *testing.T) *[]ExecReply {
	t.Helper()
	var replies []ExecReply
	prev := doLog
	doLog = func(level ReplyType, format string, args []any) {
		msg := format
		if len(args) > 0 {
			msg = fmt.Sprintf(format, args...)
		}
		replies = append(replies, ExecReply{Type: level, Message: msg})
	}
	t.Cleanup(func() { doLog = prev })
	return &replies
}

func TestProtocolOf(t *testing.T) {
	require.Equal(t, "s3", protocolOf(&Configuration{Fields: []ConfigurationField{{Key: "location", Val: "s3://host/bucket"}}}))
	require.Equal(t, "", protocolOf(&Configuration{Fields: []ConfigurationField{{Key: "other", Val: "x"}}}))
}

func TestMkimporter(t *testing.T) {
	ctx := newTestContext(t)

	// The compiled-in fs importer refuses a relative location.
	_, err := mkimporter(ctx, fsConf(uuid.Nil, "importer", "relative/path"), nil)
	require.Error(t, err)

	// An integration no plugin backs falls through to kloset's registry: tar
	// is linked in-process, an unknown protocol is not.
	tarPath := filepath.Join(t.TempDir(), "x.tar")
	require.NoError(t, os.WriteFile(tarPath, make([]byte, 1024), 0o644))
	tarConf := &Configuration{Integration: Integration{Name: "tar"}, Fields: []ConfigurationField{{Key: "location", Val: "tar://" + tarPath}}}
	imp, err := mkimporter(ctx, tarConf, nil)
	require.NoError(t, err)
	require.NoError(t, imp.Close(ctx))
	_, err = mkimporter(ctx, &Configuration{Integration: Integration{Name: "nope"}, Fields: []ConfigurationField{{Key: "location", Val: "nope://x"}}}, nil)
	require.Error(t, err)

	// A loaded plugin wins.
	loadFakePlugin(t, "importer", integration.ConnectorTypeImporter, "")
	imp, err = mkimporter(ctx, fakeConf("importer", t.TempDir()), nil)
	require.NoError(t, err)
	require.NoError(t, imp.Ping(ctx))
	require.NoError(t, imp.Close(ctx))
}

func TestMkexporter(t *testing.T) {
	ctx := newTestContext(t)

	_, err := mkexporter(ctx, fsConf(uuid.Nil, "exporter", t.TempDir(), ConfigurationField{Key: "skip_ownership", Val: "maybe"}))
	require.Error(t, err)

	_, err = mkexporter(ctx, &Configuration{Integration: Integration{Name: "nope"}, Fields: []ConfigurationField{{Key: "location", Val: "nope://x"}}})
	require.Error(t, err)

	loadFakePlugin(t, "exporter", integration.ConnectorTypeExporter, "")
	exp, err := mkexporter(ctx, fakeConf("exporter", t.TempDir()))
	require.NoError(t, err)
	require.NoError(t, exp.Ping(ctx))
	require.NoError(t, exp.Close(ctx))
}

func TestMkstorage(t *testing.T) {
	ctx := newTestContext(t)

	// No plugin for the integration.
	_, _, _, err := mkstorage(ctx, &Configuration{Integration: Integration{Name: "nope"}, Fields: []ConfigurationField{{Key: "location", Val: "nope://x"}}})
	require.ErrorIs(t, err, plugin.ErrPluginNotExist)

	// The passphrase is popped out of the params.
	st, pass, params, err := mkstorage(ctx, fsConf(uuid.Nil, "storage", t.TempDir(), ConfigurationField{Key: "passphrase", Val: "secret"}))
	require.NoError(t, err)
	require.Equal(t, "secret", pass)
	require.NotContains(t, params, "passphrase")
	require.NoError(t, st.Close(ctx))

	// A plugin-backed store.
	loadFakePlugin(t, "storage", integration.ConnectorTypeStorage, "")
	dir := t.TempDir()
	st, _, _, err = mkstorage(ctx, fakeConf("storage", dir))
	require.NoError(t, err)
	require.NoError(t, st.Close(ctx))

	// And through kloset's registry, where storage plugins are registered too.
	st, err = storage.New(ctx, map[string]string{"location": "fake://" + dir})
	require.NoError(t, err)
	require.NoError(t, st.Close(ctx))
}

func TestOpenrepo(t *testing.T) {
	ctx := newTestContext(t)

	// Store open fails: nothing at the location.
	st, _, _, err := mkstorage(ctx, fsConf(uuid.Nil, "storage", filepath.Join(t.TempDir(), "missing")))
	require.NoError(t, err)
	_, err = openrepo(ctx, st, "")
	require.Error(t, err)

	// A CONFIG that does not deserialize.
	dir := filepath.Join(t.TempDir(), "repo")
	createFSRepo(t, ctx, dir)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "CONFIG"), []byte("garbage"), 0o644))
	st, _, _, err = mkstorage(ctx, fsConf(uuid.Nil, "storage", dir))
	require.NoError(t, err)
	_, err = openrepo(ctx, st, "")
	require.Error(t, err)

	// An encrypted store: the right passphrase opens it, the wrong one trips
	// the canary.
	dir = filepath.Join(t.TempDir(), "repo")
	store := fsConf(uuid.Nil, "storage", dir, ConfigurationField{Key: "passphrase", Val: "secret"})
	_, err = dispatch(ctx, &ExecPayload{Op: "create", Source: store}, nil)
	require.NoError(t, err)
	st, pass, _, err := mkstorage(ctx, store)
	require.NoError(t, err)
	repo, err := openrepo(ctx, st, pass)
	require.NoError(t, err)
	require.NoError(t, repo.Close())
	_, err = openrepo(ctx, st, "wrong")
	require.Error(t, err)
	require.NoError(t, st.Close(ctx))

	// A KDF kloset does not know cannot derive a key.
	dir = filepath.Join(t.TempDir(), "repo")
	createFSRepoWith(t, ctx, dir, func(c *storage.Configuration) {
		c.Encryption = encryption.NewDefaultConfiguration()
		c.Encryption.KDFParams.KDF = "BOGUS"
	})
	st, _, _, err = mkstorage(ctx, fsConf(uuid.Nil, "storage", dir))
	require.NoError(t, err)
	_, err = openrepo(ctx, st, "secret")
	require.Error(t, err)
}

type failingCloser struct{}

func (failingCloser) Close(context.Context) error { return errors.New("close failed") }

func TestEnsureCloseLogsFailure(t *testing.T) {
	replies := captureLog(t)
	ensureClose(t.Context(), failingCloser{})
	require.Len(t, *replies, 1)
	require.Equal(t, ReplyWarning, (*replies)[0].Type)
	require.Contains(t, (*replies)[0].Message, "close failed")
}
