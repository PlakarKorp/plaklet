package plaklet

import (
	"fmt"
	"runtime"
	"strings"

	"github.com/PlakarKorp/kloset/connectors"
	"github.com/PlakarKorp/kloset/connectors/exporter"
	"github.com/PlakarKorp/kloset/connectors/importer"
	"github.com/PlakarKorp/kloset/connectors/storage"
	"github.com/PlakarKorp/kloset/encryption"
	"github.com/PlakarKorp/kloset/kcontext"
	"github.com/PlakarKorp/kloset/repository"
	"github.com/PlakarKorp/pkg"
	"github.com/PlakarKorp/plaklet/plugin"
)

// protocolOf extracts the protocol (scheme) from a configuration's location
// field, e.g. "s3" from "s3://host/bucket".
func protocolOf(conf *Configuration) string {
	for _, f := range conf.Fields {
		if f.Key == "location" {
			proto, _, _ := strings.Cut(f.Val, "://")
			return proto
		}
	}
	return ""
}

// mkimporter builds a source importer. It first tries a loaded plugin for the
// connector's integration; if none is registered it falls back to kloset's
// compiled-in importer registry (fs, stdio, tar).
func mkimporter(ctx *kcontext.KContext, conf *Configuration) (importer.Importer, error) {
	opts := &connectors.Options{
		Hostname:        "plaklet",
		OperatingSystem: runtime.GOOS,
		Architecture:    runtime.GOARCH,
		CWD:             "/",
		MaxConcurrency:  ctx.MaxConcurrency,
	}
	if p, err := pluginFor(conf, pkg.ConnectorTypeImporter); err == nil {
		return p.NewImporter(ctx, protocolOf(conf), conf.params(), opts)
	} else if err != plugin.ErrPluginNotExist {
		return nil, err
	}
	return importer.NewImporter(ctx, opts, conf.params())
}

// mkexporter builds a destination exporter, preferring a loaded plugin and
// falling back to kloset's compiled-in exporter registry (fs, stdio).
func mkexporter(ctx *kcontext.KContext, conf *Configuration) (exporter.Exporter, error) {
	opts := &connectors.Options{MaxConcurrency: ctx.MaxConcurrency}
	if p, err := pluginFor(conf, pkg.ConnectorTypeExporter); err == nil {
		return p.NewExporter(ctx, protocolOf(conf), conf.params(), opts)
	} else if err != plugin.ErrPluginNotExist {
		return nil, err
	}
	return exporter.NewExporter(ctx, opts, conf.params())
}

// mkstorage opens the store described by a resolved connector config and returns
// it along with its passphrase (popped out of the params) and the remaining
// store params.
func mkstorage(ctx *kcontext.KContext, conf *Configuration) (storage.Store, string, map[string]string, error) {
	params := conf.params()
	passphrase := params["passphrase"]
	delete(params, "passphrase")

	store, err := storage.New(ctx, params)
	if err != nil {
		return nil, "", nil, err
	}
	return store, passphrase, params, nil
}

// openrepo opens a kloset repository over an already-opened store. Unlike the
// plakman executor it does not go through the cached daemon: repository.New
// rebuilds the state in-process, which is what an edge (with no local cached
// daemon) needs.
func openrepo(ctx *kcontext.KContext, store storage.Store, passphrase string) (*repository.Repository, error) {
	serializedConfig, err := store.Open(ctx)
	if err != nil {
		return nil, err
	}

	repoConfig, err := storage.NewConfigurationFromWrappedBytes(serializedConfig)
	if err != nil {
		return nil, err
	}

	var key []byte
	if passphrase != "" {
		key, err = encryption.DeriveKey(repoConfig.Encryption.KDFParams, []byte(passphrase))
		if err != nil {
			return nil, err
		}
		if !encryption.VerifyCanary(repoConfig.Encryption, key) {
			return nil, fmt.Errorf("failed to verify canary; wrong passphrase?")
		}
	}

	return repository.New(ctx, key, store, serializedConfig)
}
