package connector

import (
	"context"

	sdk "github.com/PlakarKorp/integration-sdk"
)

type Factory interface {
	NewImporter(context.Context, string, map[string]string, *sdk.Options) (sdk.Importer, error)
	NewExporter(context.Context, string, map[string]string, *sdk.Options) (sdk.Exporter, error)
	NewStore(context.Context, string, map[string]string) (sdk.Store, error)
	NewInventory(context.Context, map[string]string) (sdk.Inventory, error)
	NewSecretProvider(context.Context, map[string]string) (sdk.SecretProvider, error)
}
