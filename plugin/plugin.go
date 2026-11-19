package plugin

import (
	"net"
	"os"
	"os/exec"
	"strings"

	"github.com/PlakarKorp/integration-sdk"
	"github.com/PlakarKorp/integration-sdk/location"
	"github.com/PlakarKorp/integration-sdk/stdioconn"
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

func (p *Plugin) Spawn() (net.Conn, error) {
	cmd := exec.Command(p.Exec, p.Args...)
	cmd.Stderr = os.Stderr
	return stdioconn.Spawn(cmd)
}
