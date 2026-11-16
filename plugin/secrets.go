package plugin

import (
	"context"
	"net"

	"github.com/PlakarKorp/integration-sdk"
	sdk "github.com/PlakarKorp/integration-sdk/client"
)

type SecretProviderPluginConn struct {
	integration.SecretProvider

	conn net.Conn
}

func (c *SecretProviderPluginConn) Close(ctx context.Context) error {
	return closePluginConn(ctx, "secret_provider", c.SecretProvider.Close, c.conn)
}

func (plugin *Plugin) NewSecretProvider(ctx context.Context, params map[string]string) (integration.SecretProvider, error) {
	if plugin.Type != integration.ConnectorTypeSecretProvider {
		return nil, ErrPluginTypeMismatch
	}

	client, conn, err := plugin.Connect()
	if err != nil {
		return nil, err
	}

	sp, err := sdk.NewSecretProvider(ctx, client, params)
	if err != nil {
		conn.Close()
		return nil, err
	}

	return &SecretProviderPluginConn{sp, conn}, nil
}
