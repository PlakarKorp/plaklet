package plugin

import (
	"context"
	"errors"
	"fmt"
	"io"
	"iter"
	"os"
	"os/signal"
	"syscall"
	"testing"

	"github.com/PlakarKorp/integration-sdk"
	"github.com/PlakarKorp/integration-sdk/location"
	"github.com/PlakarKorp/integration-sdk/server"
)

// The test binary doubles as the plugin executable: re-executed with
// "-fake-plugin <kind>" it serves a stub connector of that kind over its
// stdio, the way a real .ptar plugin does. The stubs only know how to start,
// answer a ping and close; a "fail" entry in the configuration makes the
// named call fail.
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
		err = server.RunImporter(func(_ context.Context, _ *integration.Options, _ string, config map[string]string) (integration.Importer, error) {
			if config["fail"] == "init" {
				return nil, errFake
			}
			return &stubImporter{stub{fail: config["fail"]}}, nil
		})
	case "exporter":
		err = server.RunExporter(func(_ context.Context, _ *integration.Options, _ string, config map[string]string) (integration.Exporter, error) {
			if config["fail"] == "init" {
				return nil, errFake
			}
			return &stubExporter{stub{fail: config["fail"]}}, nil
		})
	case "storage":
		err = server.RunStorage(func(_ context.Context, _ string, config map[string]string) (integration.Store, error) {
			if config["fail"] == "init" {
				return nil, errFake
			}
			return &stubStore{stub{fail: config["fail"]}}, nil
		})
	case "inventory":
		err = server.RunInventory(func(_ context.Context, config map[string]string) (integration.Inventory, error) {
			if config["fail"] == "init" {
				return nil, errFake
			}
			return &stubInventory{stub{fail: config["fail"]}}, nil
		})
	case "exit":
		os.Exit(1)
	}
	if err != nil && !errors.Is(err, io.EOF) {
		fmt.Fprintf(os.Stderr, "fake plugin %s: %s\n", kind, err)
	}
	os.Exit(0)
}

var errFake = errors.New("fake plugin failure")

// stub is the part every connector kind shares: identity, a ping and a close
// that fail on request.
type stub struct{ fail string }

func (stub) Origin() string        { return "fake" }
func (stub) Type() string          { return "fake" }
func (stub) Root() string          { return "/" }
func (stub) Flags() location.Flags { return 0 }
func (s stub) Ping(context.Context) error {
	if s.fail == "ping" {
		return errFake
	}
	return nil
}
func (s stub) Close(context.Context) error {
	if s.fail == "close" {
		return errFake
	}
	return nil
}

type stubImporter struct{ stub }

func (stubImporter) Import(context.Context, chan<- *integration.Record, <-chan *integration.Result) (map[string]string, error) {
	return nil, errors.ErrUnsupported
}

type stubExporter struct{ stub }

func (stubExporter) Export(context.Context, map[string]string, <-chan *integration.Record, chan<- *integration.Result) error {
	return errors.ErrUnsupported
}

type stubStore struct{ stub }

func (stubStore) Create(context.Context, []byte) error           { return errors.ErrUnsupported }
func (stubStore) Open(context.Context) ([]byte, error)           { return nil, errors.ErrUnsupported }
func (stubStore) Mode(context.Context) (integration.Mode, error) { return 0, errors.ErrUnsupported }
func (stubStore) Size(context.Context) (int64, error)            { return 0, errors.ErrUnsupported }
func (stubStore) List(context.Context, integration.StorageResource, integration.StorageFlag) iter.Seq2[*integration.StorageResourceEntry, error] {
	return func(yield func(*integration.StorageResourceEntry, error) bool) {
		yield(nil, errors.ErrUnsupported)
	}
}
func (stubStore) Put(context.Context, integration.StorageResource, integration.StorageFlag, integration.MAC, io.Reader) (int64, error) {
	return 0, errors.ErrUnsupported
}
func (stubStore) Get(context.Context, integration.StorageResource, integration.StorageFlag, integration.MAC, *integration.Range) (io.ReadCloser, error) {
	return nil, errors.ErrUnsupported
}
func (stubStore) Delete(context.Context, integration.StorageResource, integration.StorageFlag, integration.MAC) error {
	return errors.ErrUnsupported
}

type stubInventory struct{ stub }

func (stubInventory) List(_ context.Context, entries chan<- *integration.InventoryEntry) error {
	close(entries)
	return nil
}

// fakePlugin describes the fake plugin of one connector kind. exe overrides
// the executable: a path that does not exist makes the plugin fail to spawn.
func fakePlugin(t *testing.T, kind string, connType integration.ConnectorType, exe string) *Plugin {
	t.Helper()
	if exe == "" {
		var err error
		exe, err = os.Executable()
		if err != nil {
			t.Fatal(err)
		}
	}
	return &Plugin{
		Name:     "fake",
		Version:  "1.0.0",
		Type:     connType,
		Protocol: "fake",
		Exec:     exe,
		Args:     []string{"-fake-plugin", kind},
	}
}
