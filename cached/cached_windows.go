package cached

import (
	"fmt"

	"github.com/PlakarKorp/integration-sdk"
	"github.com/PlakarKorp/kloset/connectors/storage"
	"github.com/PlakarKorp/kloset/kcontext"
	"github.com/PlakarKorp/kloset/repository"
	"github.com/google/uuid"
)

func RebuildStateFromStateFile(ctx *kcontext.KContext, stateID integration.MAC, repoID uuid.UUID, storeConfig map[string]string, secret []byte, fireAndForget bool, pkgname, pkgversion string) (int, error) {
	var serializedConfig []byte
	store, serializedConfig, err := storage.Open(ctx, storeConfig)
	if err != nil {
		return -1, fmt.Errorf("failed to open storage: %w", err)
	}

	repo, err := repository.NewRepository(ctx, secret, store, serializedConfig, &repository.RepositoryOpts{RWStateCache: true})
	if err != nil {
		return -1, fmt.Errorf("failed to open repository: %w", err)
	}

	if repoID != repo.Configuration().RepositoryID {
		return -1, fmt.Errorf("invalid uuid given %q repository id is %q", repoID.String(), repo.Configuration().RepositoryID.String())
	}

	if err := repo.IngestStateFile(stateID); err != nil {
		return -1, err
	}

	return 0, nil
}

func RebuildStateFromStore(ctx *kcontext.KContext, repoID uuid.UUID, storeConfig map[string]string, secret []byte, fireAndForget bool, pkgname, pkgversion string) (int, error) {
	var serializedConfig []byte
	store, serializedConfig, err := storage.Open(ctx, storeConfig)
	if err != nil {
		return -1, fmt.Errorf("failed to open storage: %w", err)
	}

	repo, err := repository.NewRepository(ctx, secret, store, serializedConfig, &repository.RepositoryOpts{RWStateCache: true})
	if err != nil {
		return -1, fmt.Errorf("failed to open repository: %w", err)
	}

	if repoID != repo.Configuration().RepositoryID {
		return -1, fmt.Errorf("invalid uuid given %q repository id is %q", repoID.String(), repo.Configuration().RepositoryID.String())
	}

	if err := repo.RebuildState(); err != nil {
		return -1, err
	}

	return 0, nil
}
