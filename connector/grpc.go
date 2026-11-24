package connector

import (
	"context"
	"net"

	sdk "github.com/PlakarKorp/integration-sdk"
	"github.com/PlakarKorp/integration-sdk/client"
	"google.golang.org/grpc"
)

type GrpcConnector interface {
	Connect(context.Context) (grpc.ClientConnInterface, net.Conn, error)
}

type GrpcFactory struct {
	GrpcConnector
}

func (r GrpcFactory) NewImporter(ctx context.Context, proto string, params map[string]string, opts *sdk.Options) (sdk.Importer, error) {
	clt, conn, err := r.Connect(ctx)
	if err != nil {
		return nil, err
	}

	imp, err := client.NewImporter(ctx, clt, opts, proto, params)
	if err != nil {
		conn.Close()
		return nil, err
	}

	return &ImporterConn{imp, conn}, nil
}

func (r GrpcFactory) NewExporter(ctx context.Context, proto string, params map[string]string, opts *sdk.Options) (sdk.Exporter, error) {
	clt, conn, err := r.Connect(ctx)
	if err != nil {
		return nil, err
	}

	exp, err := client.NewExporter(ctx, clt, opts, proto, params)
	if err != nil {
		conn.Close()
		return nil, err
	}

	return &ExporterConn{exp, conn}, nil
}

func (r GrpcFactory) NewStore(ctx context.Context, proto string, params map[string]string) (sdk.Store, error) {
	clt, conn, err := r.Connect(ctx)
	if err != nil {
		return nil, err
	}

	st, err := client.NewStorage(ctx, clt, proto, params)
	if err != nil {
		conn.Close()
		return nil, err
	}

	return &StoreConn{st, conn}, nil
}

func (r GrpcFactory) NewInventory(ctx context.Context, params map[string]string) (sdk.Inventory, error) {
	clt, conn, err := r.Connect(ctx)
	if err != nil {
		return nil, err
	}

	inv, err := client.NewInventory(ctx, clt, params)
	if err != nil {
		conn.Close()
		return nil, err
	}

	return &InventoryConn{inv, conn}, nil
}

func (r GrpcFactory) NewSecretProvider(ctx context.Context, params map[string]string) (sdk.SecretProvider, error) {
	clt, conn, err := r.Connect(ctx)
	if err != nil {
		return nil, err
	}

	sp, err := client.NewSecretProvider(ctx, clt, params)
	if err != nil {
		conn.Close()
		return nil, err
	}

	return &SecretProviderConn{sp, conn}, nil
}
