package plugin

import (
	"bytes"
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

func TestConnectFailsToSpawn(t *testing.T) {
	p := fakePlugin(t, "importer", integration.ConnectorTypeImporter, t.TempDir()+"/missing")
	_, err := p.Spawn()
	require.Error(t, err)
}
