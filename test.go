package plaklet

import (
	"fmt"

	"github.com/PlakarKorp/kloset/kcontext"
)

func test(ctx *kcontext.KContext, input *ExecPayload) (*Report, error) {
	if input.Source == nil {
		return nil, fmt.Errorf("source must be set for test")
	}

	switch input.Source.Type {
	case "store":
		return nil, testStore(ctx, input.Source)
	case "source":
		return nil, testSource(ctx, input.Source)
	case "destination":
		return nil, testDestination(ctx, input.Source)
	default:
		return nil, fmt.Errorf("test: %s: invalid source type", input.Source.Type)
	}
}

func testStore(ctx *kcontext.KContext, target *Configuration) error {
	store, passphrase, _, err := mkstorage(ctx, target)
	if err != nil {
		return err
	}
	defer store.Close(ctx)

	repo, err := openrepo(ctx, store, passphrase)
	if err != nil {
		return err
	}

	_ = repo.Close()

	return nil
}

func testSource(ctx *kcontext.KContext, target *Configuration) error {
	importer, err := mkimporter(ctx, target)
	if err != nil {
		return err
	}
	defer importer.Close(ctx.Context) //nolint:errcheck

	return importer.Ping(ctx)
}

func testDestination(ctx *kcontext.KContext, target *Configuration) error {
	exporter, err := mkexporter(ctx, target)
	if err != nil {
		return err
	}
	defer exporter.Close(ctx.Context) //nolint:errcheck

	return exporter.Ping(ctx)
}
