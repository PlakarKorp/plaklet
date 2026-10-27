package plugin

import (
	"context"
	"net"

	sdk "github.com/PlakarKorp/integration-sdk/client"
	"github.com/PlakarKorp/kloset/connectors/storage"
	"github.com/PlakarKorp/pkg"
)

type StoragePluginConn struct {
	storage.Store

	conn net.Conn
}

func (c *StoragePluginConn) Close(ctx context.Context) error {
	err := c.Store.Close(ctx)
	errConn := c.conn.Close()
	if err != nil {
		return err
	}
	return errConn
}

func (plugin *Plugin) NewStorage(ctx context.Context, proto string, params map[string]string) (storage.Store, error) {
	if plugin.Type != pkg.ConnectorTypeStorage {
		return nil, ErrPluginTypeMismatch
	}

	client, conn, err := plugin.Connect()
	if err != nil {
		return nil, err
	}

	store, err := sdk.NewStorage(ctx, client, proto, params)
	if err != nil {
		conn.Close()
		return nil, err
	}

	return &StoragePluginConn{store, conn}, nil
}
