package plugin

import (
	"errors"
	"iter"
	"path/filepath"
	"sync"

	"github.com/PlakarKorp/pkg"
	"github.com/PlakarKorp/plaklet/plugin/logging"
)

var (
	ErrPluginExist        = errors.New("plugin already registered")
	ErrPluginNotExist     = errors.New("plugin not registered")
	ErrPluginTypeMismatch = errors.New("plugin type mismatch")
)

type Registry struct {
	plugins map[pluginKey]Plugin
	mtx     sync.Mutex
}

func NewRegistry() *Registry {
	return &Registry{
		plugins: make(map[pluginKey]Plugin),
	}
}

func (registry *Registry) ListPlugins() iter.Seq2[*Plugin, error] {
	return func(yield func(*Plugin, error) bool) {
		registry.mtx.Lock()
		defer registry.mtx.Unlock()
		for _, p := range registry.plugins {
			if !yield(&p, nil) {
				return
			}
		}
	}
}

func (registry *Registry) GetPlugin(name, version string, conntype pkg.ConnectorType, protocol string) (*Plugin, error) {
	registry.mtx.Lock()
	defer registry.mtx.Unlock()

	key := makePluginKey(name, version, conntype, protocol)
	p, found := registry.plugins[key]
	if !found {
		return nil, ErrPluginNotExist
	}
	return &p, nil
}

func (registry *Registry) RegisterPlugins(m *pkg.Manifest, p *pkg.Package, pkgdir string) error {
	registry.mtx.Lock()
	defer registry.mtx.Unlock()

	for _, conn := range m.Connectors {
		// cannot fail, this gets validated before we get here
		flags, _ := conn.Flags()

		for _, proto := range conn.Protocols {
			// NOTE: Ignoring errors as we already log in the lower layers.
			_ = registry.registerPlugin(Plugin{
				Name:     m.Name,
				Version:  p.Version,
				Type:     conn.Type,
				Protocol: proto,
				Flags:    flags,
				Exec:     filepath.Join(pkgdir, conn.Executable),
				Args:     conn.Args,
			})
		}
	}

	return nil
}

func (registry *Registry) UnregisterPlugins(m *pkg.Manifest, p *pkg.Package) {
	registry.mtx.Lock()
	defer registry.mtx.Unlock()

	for _, conn := range m.Connectors {
		for _, proto := range conn.Protocols {
			if err := registry.unregisterPlugin(makePluginKey(m.Name, p.Version, conn.Type, proto)); err != nil {
				// best-effort cleanup: a plugin that was never registered
				// (or already gone) is not worth failing the caller over.
				logging.Warn("unregister plugin: %v", err)
			}
		}
	}
}

func (registry *Registry) registerPlugin(p Plugin) error {
	_, found := registry.plugins[p.key()]
	if found {
		logging.Info("already registered plugin: %s", p.key())
		return ErrPluginExist
	}
	registry.plugins[p.key()] = p
	logging.Info("registered plugin: %s", p.key())
	return nil
}

func (registry *Registry) unregisterPlugin(key pluginKey) error {
	_, found := registry.plugins[key]
	if !found {
		return ErrPluginNotExist
	}
	logging.Info("unregistered plugin: %s", key)
	delete(registry.plugins, key)
	return nil
}
