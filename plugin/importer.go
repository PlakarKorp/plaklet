package plugin

import (
	"context"
	"net"

	"github.com/PlakarKorp/integration-sdk"
	sdk "github.com/PlakarKorp/integration-sdk/client"
)

type ImporterPluginConn struct {
	integration.Importer

	conn net.Conn
}

func (c *ImporterPluginConn) Close(ctx context.Context) error {
	return closePluginConn(ctx, "importer", c.Importer.Close, c.conn)
}

func (plugin *Plugin) NewImporter(ctx context.Context, proto string, params map[string]string, opts *integration.Options) (integration.Importer, error) {
	if plugin.Type != integration.ConnectorTypeImporter {
		return nil, ErrPluginTypeMismatch
	}

	client, conn, err := plugin.Connect()
	if err != nil {
		return nil, err
	}

	imp, err := sdk.NewImporter(ctx, client, opts, proto, params)
	if err != nil {
		conn.Close()
		return nil, err
	}

	return &ImporterPluginConn{imp, conn}, nil
}
