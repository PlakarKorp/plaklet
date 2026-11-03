package plaklet

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/PlakarKorp/integration-sdk"
	"github.com/PlakarKorp/integration-sdk/server"
	fsExporter "github.com/PlakarKorp/integrations/fs/exporter"
	fsImporter "github.com/PlakarKorp/integrations/fs/importer"
	fsStorage "github.com/PlakarKorp/integrations/fs/storage"
	"github.com/PlakarKorp/pkg"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// The test binary doubles as the plugin executable: re-executed with
// "-fake-plugin <kind>" it serves the fs connector of that kind over its
// stdio, the way a real .ptar plugin does, so the plugin code paths run
// against a genuine gRPC peer without downloading anything. A "fail" field in
// the connector configuration makes the named call fail, to reach the error
// paths a healthy connector never takes.
func TestMain(m *testing.M) {
	if len(os.Args) >= 3 && os.Args[1] == "-fake-plugin" {
		runFakePlugin(os.Args[2])
	}
	os.Exit(m.Run())
}

func runFakePlugin(kind string) {
	// The parent closes its end of the pipes once the Close call returned;
	// a frame still in flight would otherwise kill the plugin with SIGPIPE and
	// make its exit status, which the parent waits for, depend on timing.
	signal.Ignore(syscall.SIGPIPE)

	var err error
	switch kind {
	case "importer":
		err = server.RunImporter(newFakeImporter)
	case "exporter":
		err = server.RunExporter(newFakeExporter)
	case "storage":
		err = server.RunStorage(newFakeStore)
	case "inventory":
		err = server.RunInventory(newPluginInventory)
	case "exit":
		os.Exit(1)
	}
	if err != nil && !errors.Is(err, io.EOF) {
		fmt.Fprintf(os.Stderr, "fake plugin %s: %s\n", kind, err)
	}
	os.Exit(0)
}

var errFake = errors.New("fake plugin failure")

type fakeImporter struct {
	integration.Importer
	fail string
}

func newFakeImporter(ctx context.Context, opts *integration.Options, proto string, config map[string]string) (integration.Importer, error) {
	if config["fail"] == "init" {
		return nil, errFake
	}
	imp, err := fsImporter.NewFSImporter(ctx, opts, proto, config)
	if err != nil {
		return nil, err
	}
	return &fakeImporter{Importer: imp, fail: config["fail"]}, nil
}

func (f *fakeImporter) Ping(ctx context.Context) error {
	if f.fail == "ping" {
		return errFake
	}
	return f.Importer.Ping(ctx)
}

func (f *fakeImporter) Import(ctx context.Context, records chan<- *integration.Record, results <-chan *integration.Result) error {
	if f.fail == "import" {
		close(records)
		return errFake
	}
	return f.Importer.Import(ctx, records, results)
}

func (f *fakeImporter) Close(ctx context.Context) error {
	if f.fail == "close" {
		return errFake
	}
	return f.Importer.Close(ctx)
}

type fakeExporter struct {
	integration.Exporter
	fail string
}

func newFakeExporter(ctx context.Context, opts *integration.Options, proto string, config map[string]string) (integration.Exporter, error) {
	if config["fail"] == "init" {
		return nil, errFake
	}
	exp, err := fsExporter.NewFSExporter(ctx, opts, proto, config)
	if err != nil {
		return nil, err
	}
	return &fakeExporter{Exporter: exp, fail: config["fail"]}, nil
}

func (f *fakeExporter) Ping(ctx context.Context) error {
	if f.fail == "ping" {
		return errFake
	}
	return f.Exporter.Ping(ctx)
}

func (f *fakeExporter) Export(ctx context.Context, records <-chan *integration.Record, results chan<- *integration.Result) error {
	if f.fail == "export" {
		close(results)
		return errFake
	}
	return f.Exporter.Export(ctx, records, results)
}

func (f *fakeExporter) Close(ctx context.Context) error {
	if f.fail == "close" {
		return errFake
	}
	return f.Exporter.Close(ctx)
}

type fakeStore struct {
	integration.Store
	fail string
}

func newFakeStore(ctx context.Context, proto string, config map[string]string) (integration.Store, error) {
	if config["fail"] == "init" {
		return nil, errFake
	}
	st, err := fsStorage.NewStore(ctx, proto, config)
	if err != nil {
		return nil, err
	}
	return &fakeStore{Store: st, fail: config["fail"]}, nil
}

func (f *fakeStore) Open(ctx context.Context) ([]byte, error) {
	if f.fail == "open" {
		return nil, errFake
	}
	return f.Store.Open(ctx)
}

func (f *fakeStore) Put(ctx context.Context, res integration.StorageResource, mac integration.MAC, rd io.Reader) (int64, error) {
	if f.fail == "put:"+res.String() {
		return 0, errFake
	}
	return f.Store.Put(ctx, res, mac, rd)
}

func (f *fakeStore) Close(ctx context.Context) error {
	if f.fail == "close" {
		return errFake
	}
	return f.Store.Close(ctx)
}

type pluginInventory struct {
	fail string
}

func newPluginInventory(ctx context.Context, config map[string]string) (integration.Inventory, error) {
	if config["fail"] == "init" {
		return nil, errFake
	}
	return &pluginInventory{fail: config["fail"]}, nil
}

func (f *pluginInventory) List(ctx context.Context, entries chan<- *integration.InventoryEntry) error {
	defer close(entries)
	if f.fail == "list" {
		return errFake
	}
	for i := range 2 {
		entries <- &integration.InventoryEntry{
			Class: integration.ResourceClassCompute,
			URN:   fmt.Sprintf("urn:fake:%d", i),
			Name:  fmt.Sprintf("fake-%d", i),
			Endpoints: []integration.HostEndpoint{{
				Type: integration.EndpointIdentifier, Endpoint: fmt.Sprintf("fake-%d", i),
			}},
		}
	}
	return nil
}

func (f *pluginInventory) Close(context.Context) error {
	if f.fail == "close" {
		return errFake
	}
	return nil
}

// loadFakePlugin registers the fake plugin of the given connector kind under
// the "fake" protocol, through the package hooks the FlatBackend would call,
// and unloads it when the test ends. exe overrides the executable: a path
// that does not exist makes the plugin fail to spawn.
func loadFakePlugin(t *testing.T, kind string, connType integration.ConnectorType, exe string) (*pkg.Manifest, *pkg.Package) {
	t.Helper()
	if exe == "" {
		var err error
		exe, err = os.Executable()
		require.NoError(t, err)
	}
	pkgdir, name := filepath.Split(exe)
	m := &pkg.Manifest{
		Name: "fake",
		Connectors: []pkg.ManifestConnector{{
			Type:          connType,
			Protocols:     []string{"fake"},
			LocationFlags: []string{"localfs"},
			Executable:    name,
			Args:          []string{"-fake-plugin", kind},
		}},
	}
	p := &pkg.Package{Name: "fake", Version: "1.0.0", OperatingSystem: "test", Architecture: "test"}
	pkgLoadHook(m, p, pkgdir)
	t.Cleanup(func() { pkgUnloadHook(m, p) })
	return m, p
}

// fakeConf is a connector configuration backed by the fake plugin; extra
// fields are passed to the connector verbatim.
func fakeConf(typ, location string, extra ...ConfigurationField) *Configuration {
	conf := &Configuration{
		Id:          uuid.MustParse("55555555-5555-5555-5555-555555555555"),
		Type:        typ,
		Integration: Integration{Name: "fake", Version: "1.0.0"},
		Name:        typ,
		Fields:      []ConfigurationField{{Key: "location", Val: "fake://" + location}},
	}
	conf.Fields = append(conf.Fields, extra...)
	return conf
}

func failField(what string) ConfigurationField {
	return ConfigurationField{Key: "fail", Val: what}
}
