package plugin

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"

	"github.com/PlakarKorp/integration-sdk/stdioconn"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func ConnectPlugin(ctx context.Context, pluginPath string, args []string) (grpc.ClientConnInterface, net.Conn, error) {
	cmd := exec.CommandContext(ctx, pluginPath, args...)
	cmd.Stderr = os.Stderr
	conn, err := stdioconn.Spawn(cmd)
	if err != nil {
		return nil, nil, err
	}

	clientConn, err := grpc.NewClient("127.0.0.1:0",
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
