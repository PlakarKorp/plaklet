package plaklet

import (
	"testing"

	"github.com/PlakarKorp/integration-sdk"
	"github.com/PlakarKorp/kloset/connectors/storage"
	"github.com/stretchr/testify/require"
)

// A storage protocol registered with kloset resolves its plugin at every
// open: once the plugin is gone from the registry, opening fails instead of
// dispatching to nothing.
func TestStorageRegistrationResolvesPluginLate(t *testing.T) {
	ctx := newTestContext(t)
	m, p := loadFakePlugin(t, "storage", integration.ConnectorTypeStorage, "")

	pluginRegistry.UnregisterPlugins(m, p)
	_, err := storage.New(ctx, map[string]string{"location": "fake://" + t.TempDir()})
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to get plugin")

	_, err = pluginFor(fakeConf("storage", t.TempDir()), integration.ConnectorTypeStorage)
	require.Error(t, err)
}
