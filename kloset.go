package main

import (
	"fmt"
	"runtime"

	"github.com/PlakarKorp/kloset/connectors"
	"github.com/PlakarKorp/kloset/connectors/exporter"
	"github.com/PlakarKorp/kloset/connectors/importer"
	"github.com/PlakarKorp/kloset/connectors/storage"
	"github.com/PlakarKorp/kloset/encryption"
	"github.com/PlakarKorp/kloset/kcontext"
	"github.com/PlakarKorp/kloset/repository"
)

// mkimporter builds a source importer from a resolved connector configuration
// using kloset's in-process connector registry (see connectors.go for the
// registered backends).
func mkimporter(ctx *kcontext.KContext, conf *Configuration) (importer.Importer, error) {
	return importer.NewImporter(ctx, &connectors.Options{
		Hostname:        "plaklet",
		OperatingSystem: runtime.GOOS,
		Architecture:    runtime.GOARCH,
		CWD:             "/",
		MaxConcurrency:  ctx.MaxConcurrency,
	}, conf.params())
}

// mkexporter builds a destination exporter from a resolved connector config.
func mkexporter(ctx *kcontext.KContext, conf *Configuration) (exporter.Exporter, error) {
	return exporter.NewExporter(ctx, &connectors.Options{
		MaxConcurrency: ctx.MaxConcurrency,
	}, conf.params())
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
