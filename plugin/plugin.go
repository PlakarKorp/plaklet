package plugin

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"

	"github.com/PlakarKorp/integration-sdk"
	"github.com/PlakarKorp/integration-sdk/location"
	"github.com/PlakarKorp/integration-sdk/stdioconn"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type Plugin struct {
	Name     string
	Version  string
	Type     integration.ConnectorType
	Protocol string
	Flags    location.Flags
	Exec     string
	Args     []string
}

type PluginConnCloseError struct {
	Name         string
	CloseErr     error
	ConnCloseErr error
}

type pluginKey string

func makePluginKey(name, version string, conntype integration.ConnectorType, protocol string) pluginKey {
	var b strings.Builder
	b.WriteString(name)
	b.WriteRune(':')
	b.WriteString(version)
	b.WriteRune(':')
	b.WriteString(string(conntype))
	b.WriteRune(':')

	// Hack for now: For inventory and secret provider plugins, the protocol is
	// just here for the manifest to be valid. We have to ignore it here for them
	// to work. This probably has to be fixed at somt point.
	if conntype == integration.ConnectorTypeSecretProvider || conntype == integration.ConnectorTypeInventory {
		protocol = ""
	}
	b.WriteString(protocol)
	return pluginKey(b.String())
}

func (p *Plugin) key() pluginKey {
	return makePluginKey(p.Name, p.Version, p.Type, p.Protocol)
}

func (p *Plugin) Connect() (grpc.ClientConnInterface, net.Conn, error) {
	cmd := exec.Command(p.Exec, p.Args...)
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

func (e PluginConnCloseError) Error() string {
	closeErr := "ok"
	connErr := "ok"
	if e.CloseErr != nil {
		closeErr = e.CloseErr.Error()
	}
	if e.ConnCloseErr != nil {
		connErr = e.ConnCloseErr.Error()
	}
	return fmt.Sprintf("%s: [close:%s] [conn:%s]", e.Name, closeErr, connErr)
}

func closePluginConn(ctx context.Context, name string, closeFn func(context.Context) error, conn net.Conn) error {
	var err PluginConnCloseError
	err.Name = name
	err.CloseErr = closeFn(ctx)
	err.ConnCloseErr = conn.Close()
	if err.CloseErr != nil || err.ConnCloseErr != nil {
		return err
	}
	return nil
}
