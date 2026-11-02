package plugin

import (
	"context"
	"net"

	"github.com/PlakarKorp/integration-sdk"
	sdk "github.com/PlakarKorp/integration-sdk/client"
)

type InventoryPluginConn struct {
	integration.Inventory

	conn net.Conn
}

func (c *InventoryPluginConn) Close(ctx context.Context) error {
	return closePluginConn(ctx, "inventory", c.Inventory.Close, c.conn)
}

func (plugin *Plugin) NewInventory(ctx context.Context, proto string, params map[string]string) (integration.Inventory, error) {
	if plugin.Type != integration.ConnectorTypeInventory {
		return nil, ErrPluginTypeMismatch
	}

	client, conn, err := plugin.Connect()
	if err != nil {
		return nil, err
	}

	imp, err := sdk.NewInventory(ctx, client, params)
	if err != nil {
		conn.Close()
		return nil, err
	}

	return &InventoryPluginConn{imp, conn}, nil
}
