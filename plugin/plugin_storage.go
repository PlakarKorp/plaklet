package plugin

import (
	"context"

	grpc_storage "github.com/PlakarKorp/integration-grpc/storage"
	"github.com/PlakarKorp/kloset/connectors/storage"
	"github.com/PlakarKorp/kloset/kcontext"
	"github.com/PlakarKorp/pkg"
)

type StoragePluginConn struct {
	storage.Store

	conn ExitConn
}

func (c *StoragePluginConn) Close(ctx context.Context) error {
	err := c.Store.Close(ctx)
	errConn := c.conn.Close()
	if err != nil {
		return err
	}
	return errConn
}

func (plugin *Plugin) NewStorage(ktx *kcontext.KContext, proto string, params map[string]string) (storage.Store, error) {
	if plugin.Type != pkg.ConnectorTypeStorage {
		return nil, ErrPluginTypeMismatch
	}

	client, conn, err := plugin.Connect(ktx.Context)
	if err != nil {
		return nil, err
	}

	store, err := grpc_storage.NewStorage(ktx, client, proto, params)
	if err != nil {
		conn.Close()
		return nil, err
	}

	return &StoragePluginConn{store, conn}, nil
}
