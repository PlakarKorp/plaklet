package plugin

import (
	"bytes"
	"errors"
	"log"
	"testing"

	"github.com/PlakarKorp/integration-sdk"
	"github.com/PlakarKorp/pkg"
	"github.com/stretchr/testify/require"
)

func TestPluginKey(t *testing.T) {
	require.Equal(t, pluginKey("s3:1.0.0:storage:s3"), makePluginKey("s3", "1.0.0", integration.ConnectorTypeStorage, "s3"))
	// Inventory and secret providers are keyed without their protocol.
	require.Equal(t, pluginKey("scw:1.0.0:inventory:"), makePluginKey("scw", "1.0.0", integration.ConnectorTypeInventory, "scw"))
	require.Equal(t, pluginKey("vault:1.0.0:secret_provider:"), makePluginKey("vault", "1.0.0", integration.ConnectorTypeSecretProvider, "vault"))
	p := &Plugin{Name: "s3", Version: "1.0.0", Type: integration.ConnectorTypeImporter, Protocol: "s3"}
	require.Equal(t, pluginKey("s3:1.0.0:importer:s3"), p.key())
}

func TestRegistry(t *testing.T) {
	var buf bytes.Buffer
	prev := log.Writer()
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(prev) })

	r := NewRegistry()
	m := &pkg.Manifest{
		Name: "s3",
		Connectors: []pkg.ManifestConnector{
			{Type: integration.ConnectorTypeStorage, Protocols: []string{"s3", "s3+http"}, Executable: "s3-storage"},
			{Type: integration.ConnectorTypeImporter, Protocols: []string{"s3"}, Executable: "s3-importer", Args: []string{"-v"}},
		},
	}
	p := &pkg.Package{Name: "s3", Version: "1.0.0"}
	require.NoError(t, r.RegisterPlugins(m, p, "/pkgs"))
	// Registering again is a no-op, not an error for the caller.
	require.NoError(t, r.RegisterPlugins(m, p, "/pkgs"))

	got, err := r.GetPlugin("s3", "1.0.0", integration.ConnectorTypeImporter, "s3")
	require.NoError(t, err)
	require.Equal(t, "/pkgs/s3-importer", got.Exec)
	require.Equal(t, []string{"-v"}, got.Args)
	_, err = r.GetPlugin("s3", "2.0.0", integration.ConnectorTypeImporter, "s3")
	require.ErrorIs(t, err, ErrPluginNotExist)

	var all int
	for p, err := range r.ListPlugins() {
		require.NoError(t, err)
		require.Equal(t, "s3", p.Name)
		all++
	}
	require.Equal(t, 3, all)
	for range r.ListPlugins() {
		break // stopping early is fine
	}

	r.UnregisterPlugins(m, p)
	_, err = r.GetPlugin("s3", "1.0.0", integration.ConnectorTypeStorage, "s3")
	require.ErrorIs(t, err, ErrPluginNotExist)
	// Unregistering what is already gone is logged, not fatal.
	r.UnregisterPlugins(m, p)
	require.Contains(t, buf.String(), "unregister plugin")
}

func TestPluginConnCloseError(t *testing.T) {
	require.Equal(t, "x: [close:ok] [conn:ok]", PluginConnCloseError{Name: "x"}.Error())
	require.Equal(t, "x: [close:a] [conn:b]", PluginConnCloseError{Name: "x", CloseErr: errors.New("a"), ConnCloseErr: errors.New("b")}.Error())
}

func TestConnectFailsToSpawn(t *testing.T) {
	p := fakePlugin(t, "importer", integration.ConnectorTypeImporter, t.TempDir()+"/missing")
	_, _, err := p.Connect()
	require.Error(t, err)
}

func TestNewImporter(t *testing.T) {
	ctx := t.Context()
	opts := &integration.Options{MaxConcurrency: 1}

	_, err := fakePlugin(t, "importer", integration.ConnectorTypeExporter, "").NewImporter(ctx, "fake", nil, opts)
	require.ErrorIs(t, err, ErrPluginTypeMismatch)
	_, err = fakePlugin(t, "importer", integration.ConnectorTypeImporter, "/missing").NewImporter(ctx, "fake", nil, opts)
	require.Error(t, err)
	_, err = fakePlugin(t, "exit", integration.ConnectorTypeImporter, "").NewImporter(ctx, "fake", nil, opts)
	require.Error(t, err, "a plugin that dies before answering")
	_, err = fakePlugin(t, "importer", integration.ConnectorTypeImporter, "").NewImporter(ctx, "fake", map[string]string{"fail": "init"}, opts)
	require.Error(t, err)

	imp, err := fakePlugin(t, "importer", integration.ConnectorTypeImporter, "").NewImporter(ctx, "fake", nil, opts)
	require.NoError(t, err)
	require.NoError(t, imp.Ping(ctx))
	require.NoError(t, imp.Close(ctx))

	imp, err = fakePlugin(t, "importer", integration.ConnectorTypeImporter, "").NewImporter(ctx, "fake", map[string]string{"fail": "close"}, opts)
	require.NoError(t, err)
	err = imp.Close(ctx)
	var closeErr PluginConnCloseError
	require.ErrorAs(t, err, &closeErr)
	require.Equal(t, "importer", closeErr.Name)
}

func TestNewExporter(t *testing.T) {
	ctx := t.Context()
	opts := &integration.Options{MaxConcurrency: 1}

	_, err := fakePlugin(t, "exporter", integration.ConnectorTypeImporter, "").NewExporter(ctx, "fake", nil, opts)
	require.ErrorIs(t, err, ErrPluginTypeMismatch)
	_, err = fakePlugin(t, "exporter", integration.ConnectorTypeExporter, "/missing").NewExporter(ctx, "fake", nil, opts)
	require.Error(t, err)
	_, err = fakePlugin(t, "exporter", integration.ConnectorTypeExporter, "").NewExporter(ctx, "fake", map[string]string{"fail": "init"}, opts)
	require.Error(t, err)

	exp, err := fakePlugin(t, "exporter", integration.ConnectorTypeExporter, "").NewExporter(ctx, "fake", nil, opts)
	require.NoError(t, err)
	require.NoError(t, exp.Ping(ctx))
	require.NoError(t, exp.Close(ctx))

	exp, err = fakePlugin(t, "exporter", integration.ConnectorTypeExporter, "").NewExporter(ctx, "fake", map[string]string{"fail": "close"}, opts)
	require.NoError(t, err)
	require.Error(t, exp.Close(ctx))
}

func TestNewStorage(t *testing.T) {
	ctx := t.Context()

	_, err := fakePlugin(t, "storage", integration.ConnectorTypeImporter, "").NewStorage(ctx, "fake", nil)
	require.ErrorIs(t, err, ErrPluginTypeMismatch)
	_, err = fakePlugin(t, "storage", integration.ConnectorTypeStorage, "/missing").NewStorage(ctx, "fake", nil)
	require.Error(t, err)
	_, err = fakePlugin(t, "storage", integration.ConnectorTypeStorage, "").NewStorage(ctx, "fake", map[string]string{"fail": "init"})
	require.Error(t, err)

	st, err := fakePlugin(t, "storage", integration.ConnectorTypeStorage, "").NewStorage(ctx, "fake", nil)
	require.NoError(t, err)
	require.NoError(t, st.Ping(ctx))
	require.NoError(t, st.Close(ctx))

	st, err = fakePlugin(t, "storage", integration.ConnectorTypeStorage, "").NewStorage(ctx, "fake", map[string]string{"fail": "close"})
	require.NoError(t, err)
	require.Error(t, st.Close(ctx))
}

func TestNewInventory(t *testing.T) {
	ctx := t.Context()

	_, err := fakePlugin(t, "inventory", integration.ConnectorTypeImporter, "").NewInventory(ctx, "fake", nil)
	require.ErrorIs(t, err, ErrPluginTypeMismatch)
	_, err = fakePlugin(t, "inventory", integration.ConnectorTypeInventory, "/missing").NewInventory(ctx, "fake", nil)
	require.Error(t, err)
	_, err = fakePlugin(t, "inventory", integration.ConnectorTypeInventory, "").NewInventory(ctx, "fake", map[string]string{"fail": "init"})
	require.Error(t, err)

	inv, err := fakePlugin(t, "inventory", integration.ConnectorTypeInventory, "").NewInventory(ctx, "fake", nil)
	require.NoError(t, err)
	entries := make(chan *integration.InventoryEntry)
	go func() {
		for range entries {
		}
	}()
	require.NoError(t, inv.List(ctx, entries))
	require.NoError(t, inv.Close(ctx))

	inv, err = fakePlugin(t, "inventory", integration.ConnectorTypeInventory, "").NewInventory(ctx, "fake", map[string]string{"fail": "close"})
	require.NoError(t, err)
	require.Error(t, inv.Close(ctx))
}
