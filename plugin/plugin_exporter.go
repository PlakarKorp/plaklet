package plugin

import (
	"context"

	grpc_exporter "github.com/PlakarKorp/integration-grpc/exporter"
	"github.com/PlakarKorp/kloset/connectors"
	"github.com/PlakarKorp/kloset/connectors/exporter"
	"github.com/PlakarKorp/kloset/kcontext"
	"github.com/PlakarKorp/pkg"
)

type ExporterPluginConn struct {
	exporter.Exporter

	conn ExitConn
}

func (c *ExporterPluginConn) Close(ctx context.Context) error {
	err := c.Exporter.Close(ctx)
	errConn := c.conn.Close()
	if err != nil {
		return err
	}
	return errConn
}

func (plugin *Plugin) NewExporter(ktx *kcontext.KContext, proto string, params map[string]string, opts *connectors.Options) (exporter.Exporter, error) {
	if plugin.Type != pkg.ConnectorTypeExporter {
		return nil, ErrPluginTypeMismatch
	}

	client, conn, err := plugin.Connect(ktx.Context)
	if err != nil {
		return nil, err
	}

	exp, err := grpc_exporter.NewExporter(ktx, client, opts, proto, params)
	if err != nil {
		conn.Close()
		return nil, err
	}

	return &ExporterPluginConn{exp, conn}, nil
}
