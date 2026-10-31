package plugin

import (
	"context"
	"net"

	"github.com/PlakarKorp/integration-sdk"
	sdk "github.com/PlakarKorp/integration-sdk/client"
)

type ExporterPluginConn struct {
	integration.Exporter

	conn net.Conn
}

func (c *ExporterPluginConn) Close(ctx context.Context) error {
	return closePluginConn(ctx, "importer", c.Exporter.Close, c.conn)
}

func (plugin *Plugin) NewExporter(ctx context.Context, proto string, params map[string]string, opts *integration.Options) (integration.Exporter, error) {
	if plugin.Type != integration.ConnectorTypeExporter {
		return nil, ErrPluginTypeMismatch
	}

	client, conn, err := plugin.Connect()
	if err != nil {
		return nil, err
	}

	exp, err := sdk.NewExporter(ctx, client, opts, proto, params)
	if err != nil {
		conn.Close()
		return nil, err
	}

	return &ExporterPluginConn{exp, conn}, nil
}
