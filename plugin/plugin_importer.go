package plugin

import (
	"context"

	grpc_importer "github.com/PlakarKorp/integration-grpc/importer"
	"github.com/PlakarKorp/kloset/connectors"
	"github.com/PlakarKorp/kloset/connectors/importer"
	"github.com/PlakarKorp/kloset/kcontext"
	"github.com/PlakarKorp/pkg"
)

type ImporterPluginConn struct {
	importer.Importer

	conn ExitConn
}

func (c *ImporterPluginConn) Close(ctx context.Context) error {
	err := c.Importer.Close(ctx)
	errConn := c.conn.Close()
	if err != nil {
		return err
	}
	return errConn
}

func (plugin *Plugin) NewImporter(ktx *kcontext.KContext, proto string, params map[string]string, opts *connectors.Options) (importer.Importer, error) {
	if plugin.Type != pkg.ConnectorTypeImporter {
		return nil, ErrPluginTypeMismatch
	}

	client, conn, err := plugin.Connect(ktx.Context)
	if err != nil {
		return nil, err
	}

	imp, err := grpc_importer.NewImporter(ktx, client, opts, proto, params)
	if err != nil {
		conn.Close()
		return nil, err
	}

	return &ImporterPluginConn{imp, conn}, nil
}
