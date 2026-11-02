package plugin

import (
	"context"
	"net"

	"github.com/PlakarKorp/integration-sdk"
	sdk "github.com/PlakarKorp/integration-sdk/client"
)

type StoragePluginConn struct {
	integration.Store

	conn net.Conn
}

func (c *StoragePluginConn) Close(ctx context.Context) error {
	return closePluginConn(ctx, "store", c.Store.Close, c.conn)
}

func (plugin *Plugin) NewStorage(ctx context.Context, proto string, params map[string]string) (integration.Store, error) {
	if plugin.Type != integration.ConnectorTypeStorage {
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
