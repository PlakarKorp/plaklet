package plaklet

import (
	"context"
	"fmt"

	"github.com/PlakarKorp/integration-sdk"
	"github.com/PlakarKorp/integration-sdk/location"
	"github.com/PlakarKorp/kloset/connectors/storage"
	"github.com/PlakarKorp/pkg"
	"github.com/PlakarKorp/plaklet/plugin"
)

// pluginRegistry holds the out-of-process connector plugins loaded from .ptar
// packages. Storage plugins are additionally registered into kloset's storage
// registry (so storage.New finds them); importer/exporter plugins are resolved
// directly through the registry in mkimporter/mkexporter. This mirrors the
// hybrid model plakman's executor uses.
var pluginRegistry = plugin.NewRegistry()

// storage.New(proto, ...) dispatches to the gRPC plugin.
func registerStorage(name, version, proto string, flags location.Flags) error {
	return storage.Register(proto, flags, func(ctx context.Context, s string, config map[string]string) (integration.Store, error) {
		p, err := pluginRegistry.GetPlugin(name, version, integration.ConnectorTypeStorage, proto)
		if err != nil {
			return nil, fmt.Errorf("failed to get plugin: %w", err)
		}
		return p.NewStorage(ctx, proto, config)
	})
}

// pkgLoadHook is called by the FlatBackend for each loaded package: it records
// every connector in the registry and registers storage connectors with kloset.
func pkgLoadHook(m *pkg.Manifest, p *pkg.Package, pkgdir string) {
	if err := pluginRegistry.RegisterPlugins(m, p, pkgdir); err != nil {
		// best-effort; lower layers log
		_ = err
	}
	for _, conn := range m.Connectors {
		for _, proto := range conn.Protocols {
			if conn.Type == integration.ConnectorTypeStorage {
				flags, _ := conn.Flags()
				_ = registerStorage(m.Name, p.Version, proto, flags)
			}
		}
	}
}

func pkgUnloadHook(m *pkg.Manifest, p *pkg.Package) {
	pluginRegistry.UnregisterPlugins(m, p)
	for _, conn := range m.Connectors {
		for _, proto := range conn.Protocols {
			if conn.Type == integration.ConnectorTypeStorage {
				storage.Unregister(proto)
			}
		}
	}
}

// pluginFor resolves the loaded plugin backing a connector configuration, by its
// integration name/version, connector type, and protocol (from the location).
func pluginFor(conf *Configuration, connType integration.ConnectorType) (*plugin.Plugin, error) {
	return pluginRegistry.GetPlugin(conf.Integration.Name, conf.Integration.Version, connType, protocolOf(conf))
}
