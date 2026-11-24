package connector

import (
	"context"
	"fmt"
	"net"

	"github.com/PlakarKorp/plaklet/plugin"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func ForPlugin(p *plugin.Plugin) Factory {
	return GrpcFactory{PluginConnector{p}}
}

type PluginConnector struct {
	p *plugin.Plugin
}

func (c PluginConnector) Connect(ctx context.Context) (grpc.ClientConnInterface, net.Conn, error) {
	conn, err := c.p.Spawn()
	if err != nil {
		return nil, nil, err
	}

	clientConn, err := grpc.NewClient("passthrough://plugin",
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithIdleTimeout(0),
		grpc.WithContextDialer(func(ctx context.Context, s string) (net.Conn, error) {
			return conn, nil
		}),
	)
	if err != nil {
		conn.Close()
		return nil, nil, fmt.Errorf("grpc client creation failed: %w", err)
	}

	return clientConn, conn, nil
}
