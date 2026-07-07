package plugin

import (
	"context"
	"strings"

	"github.com/PlakarKorp/kloset/location"
	"github.com/PlakarKorp/pkg"
	"google.golang.org/grpc"
)

type Plugin struct {
	Name     string
	Version  string
	Type     pkg.ConnectorType
	Protocol string
	Flags    location.Flags
	Exec     string
	Args     []string
}

type pluginKey string

func makePluginKey(name, version string, conntype pkg.ConnectorType, protocol string) pluginKey {
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
	if conntype == pkg.ConnectorTypeSecretProvider || conntype == pkg.ConnectorTypeInventory {
		protocol = ""
	}
	b.WriteString(protocol)
	return pluginKey(b.String())
}

func (p *Plugin) key() pluginKey {
	return makePluginKey(p.Name, p.Version, p.Type, p.Protocol)
}

func (p *Plugin) Connect(ctx context.Context) (grpc.ClientConnInterface, ExitConn, error) {
	return ConnectPlugin(ctx, p.Exec, p.Args)
}
