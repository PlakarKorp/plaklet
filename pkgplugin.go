package plaklet

import (
	"context"
	"fmt"

	grpc_storage "github.com/PlakarKorp/integration-grpc/storage"
	"github.com/PlakarKorp/kloset/connectors/storage"
	"github.com/PlakarKorp/kloset/location"
	"github.com/PlakarKorp/pkg"
	"github.com/PlakarKorp/plaklet/plugin"
)

// pluginRegistry holds the out-of-process connector plugins loaded from .ptar
// packages. Storage plugins are additionally registered into kloset's storage
// registry (so storage.New finds them); importer/exporter plugins are resolved
// directly through the registry in mkimporter/mkexporter. This mirrors the
// hybrid model plakman's executor uses.
var pluginRegistry = plugin.NewRegistry()

// registerStorage wires a storage plugin into kloset's storage registry so
// storage.New(proto, ...) dispatches to the gRPC plugin.
func registerStorage(name, version, proto string, flags location.Flags) error {
	return storage.Register(proto, flags, func(ctx context.Context, s string, config map[string]string) (storage.Store, error) {
		p, err := pluginRegistry.GetPlugin(name, version, pkg.ConnectorTypeStorage, proto)
		if err != nil {
			return nil, fmt.Errorf("failed to get plugin: %w", err)
		}
		client, _, err := p.Connect(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to connect to plugin: %w", err)
		}
		return grpc_storage.NewStorage(ctx, client, s, config)
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
			if conn.Type == pkg.ConnectorTypeStorage {
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
			if conn.Type == pkg.ConnectorTypeStorage {
				storage.Unregister(proto)
			}
		}
	}
}

// pluginFor resolves the loaded plugin backing a connector configuration, by its
// integration name/version, connector type, and protocol (from the location).
func pluginFor(conf *Configuration, connType pkg.ConnectorType) (*plugin.Plugin, error) {
	proto := protocolOf(conf)
	if proto == "" {
		return nil, fmt.Errorf("could not determine protocol for %s", conf.Integration.Name)
	}
	return pluginRegistry.GetPlugin(conf.Integration.Name, conf.Integration.Version, connType, proto)
}
