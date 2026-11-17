//go:build !windows

package cached

import (
	"github.com/PlakarKorp/integration-sdk"
	"github.com/PlakarKorp/kloset/kcontext"
	"github.com/google/uuid"
)

func RebuildStateFromStateFile(ctx *kcontext.KContext, stateID integration.MAC, repoID uuid.UUID, storeConfig map[string]string, secret []byte, fireAndForget bool, pkgname, pkgversion string) (int, error) {
	req := &RequestPkt{
		PluginName:    pkgname,
		PluginVersion: pkgversion,
		Secret:        secret,
		RepoID:        repoID,
		StoreConfig:   storeConfig,
		StateID:       stateID,
		FireAndForget: fireAndForget,
	}

	return rebuildStateRequest(ctx, req)
}

func RebuildStateFromStore(ctx *kcontext.KContext, repoID uuid.UUID, storeConfig map[string]string, secret []byte, fireAndForget bool, pkgname, pkgversion string) (int, error) {
	req := &RequestPkt{
		PluginName:    pkgname,
		PluginVersion: pkgversion,
		Secret:        secret,
		RepoID:        repoID,
		StoreConfig:   storeConfig,
		FireAndForget: fireAndForget,
	}

	return rebuildStateRequest(ctx, req)
}
