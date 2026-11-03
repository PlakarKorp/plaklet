package plaklet

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/PlakarKorp/integration-sdk"
	"github.com/PlakarKorp/kloset/events"
	"github.com/PlakarKorp/plaklet/plugin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// runMain runs Main with its standard streams swapped for files: stdin holds
// the payload, stdout collects the replies. stderr goes to a file too, so the
// test output is not littered with kloset's logging.
func runMain(t *testing.T, args []string, stdin string) (int, []ExecReply) {
	t.Helper()
	dir := t.TempDir()
	in, err := os.Create(filepath.Join(dir, "stdin"))
	require.NoError(t, err)
	_, err = in.WriteString(stdin)
	require.NoError(t, err)
	_, err = in.Seek(0, 0)
	require.NoError(t, err)
	out, err := os.Create(filepath.Join(dir, "stdout"))
	require.NoError(t, err)
	errf, err := os.Create(filepath.Join(dir, "stderr"))
	require.NoError(t, err)

	prevIn, prevOut, prevErr, prevLog := os.Stdin, os.Stdout, os.Stderr, log.Writer()
	os.Stdin, os.Stdout, os.Stderr = in, out, errf
	log.SetOutput(errf)
	defer func() {
		os.Stdin, os.Stdout, os.Stderr = prevIn, prevOut, prevErr
		log.SetOutput(prevLog)
		in.Close()
		out.Close()
		errf.Close()
	}()

	// Keep the test process's GOMAXPROCS as it is.
	args = append([]string{"-cpu", strconv.Itoa(runtime.GOMAXPROCS(0))}, args...)
	code := Main(args)

	raw, err := os.ReadFile(out.Name())
	require.NoError(t, err)
	var replies []ExecReply
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		if line == "" {
			continue
		}
		var r ExecReply
		require.NoError(t, json.Unmarshal([]byte(line), &r), line)
		replies = append(replies, r)
	}
	return code, replies
}

func payload(t *testing.T, p *ExecPayload) string {
	t.Helper()
	raw, err := json.Marshal(p)
	require.NoError(t, err)
	return string(raw)
}

func lastReply(replies []ExecReply) ExecReply {
	return replies[len(replies)-1]
}

func TestMainUsage(t *testing.T) {
	code, _ := runMain(t, []string{"-no-such-flag"}, "")
	require.Equal(t, 2, code)

	code, _ = runMain(t, nil, "")
	require.Equal(t, 2, code, "-cache is required")

	file := filepath.Join(t.TempDir(), "file")
	require.NoError(t, os.WriteFile(file, nil, 0o600))
	code, _ = runMain(t, []string{"-cache", filepath.Join(file, "cache")}, "")
	require.Equal(t, 1, code, "cache dir cannot be created")
}

func TestMainPackages(t *testing.T) {
	cache := t.TempDir()
	file := filepath.Join(t.TempDir(), "file")
	require.NoError(t, os.WriteFile(file, nil, 0o600))

	// The package directory cannot be created.
	code, _ := runMain(t, []string{"-cache", cache, "-pkg", filepath.Join(file, "pkg")}, "")
	require.Equal(t, 1, code)

	// A package that is not a ptar cannot be loaded.
	pkgdir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(pkgdir, "bogus_v1.0.0_linux_amd64.ptar"), []byte("not a ptar"), 0o644))
	code, _ = runMain(t, []string{"-cache", cache, "-pkg", pkgdir}, "")
	require.Equal(t, 1, code)

	// An empty package directory loads nothing; with no payload on stdin
	// there is nothing to do.
	code, replies := runMain(t, []string{"-cache", cache, "-pkg", t.TempDir()}, "")
	require.Equal(t, 0, code)
	require.Empty(t, replies)
}

func TestMainPayload(t *testing.T) {
	cache := t.TempDir()

	// Not JSON.
	code, _ := runMain(t, []string{"-cache", cache}, "{not json")
	require.Equal(t, 1, code)

	// An operation that fails ends with a failure reply, after the state
	// samples, and the process still exits 0: the verdict is in the stream.
	code, replies := runMain(t, []string{"-cache", cache, "-quiet"}, payload(t, &ExecPayload{Op: "bogus"}))
	require.Equal(t, 0, code)
	require.Equal(t, ReplyFailure, lastReply(replies).Type)
	require.Contains(t, lastReply(replies).Message, "bogus failed")
	var states int
	for _, r := range replies {
		if r.Type == ReplyState {
			states++
			var st State
			require.NoError(t, json.Unmarshal(r.State, &st))
		}
	}
	require.Positive(t, states, "the final state sample is always sent")

	// An operation without a report ends with success alone.
	store := fsConf(uuid.MustParse("22222222-2222-2222-2222-222222222222"), "storage", filepath.Join(t.TempDir(), "repo"))
	code, replies = runMain(t, []string{"-cache", cache, "-concurrency", "2"}, payload(t, &ExecPayload{Op: "create", Source: store}))
	require.Equal(t, 0, code)
	require.Equal(t, ReplySuccess, lastReply(replies).Type)
	for _, r := range replies {
		require.NotEqual(t, ReplyReport, r.Type)
	}

	// An operation with a report sends it right before success.
	code, replies = runMain(t, []string{"-cache", cache}, payload(t, &ExecPayload{Op: "rm", Source: store}))
	require.Equal(t, 0, code)
	require.Equal(t, ReplySuccess, lastReply(replies).Type)
	report := replies[len(replies)-2]
	require.Equal(t, ReplyReport, report.Type)
	var rep Report
	require.NoError(t, json.Unmarshal(report.Report, &rep))
	require.NotNil(t, rep.Rm)

	// A backup that could not read everything is a failure after the fact.
	srcDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "a.txt"), []byte("hello"), 0o644))
	source := fsConf(uuid.MustParse("11111111-1111-1111-1111-111111111111"), "importer", srcDir)
	code, replies = runMain(t, []string{"-cache", cache}, payload(t, &ExecPayload{Op: "backup", Source: source, Target: store}))
	require.Equal(t, 0, code)
	require.Equal(t, ReplySuccess, lastReply(replies).Type)
}

// TestStateSampler runs the sampler long enough for its ticker to fire, then
// stops it and checks every reply carries a decodable state.
func TestStateSampler(t *testing.T) {
	bus := events.NewEventsBUS(16)
	listener := NewEventListener(true)
	listener.Run(bus)
	t.Cleanup(func() { bus.Close(); listener.Wait() })

	var mu sync.Mutex
	var replies []*ExecReply
	send := func(r *ExecReply) {
		mu.Lock()
		replies = append(replies, r)
		mu.Unlock()
	}
	count := func() int {
		mu.Lock()
		defer mu.Unlock()
		return len(replies)
	}

	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Go(StateSampler("backup", listener, stop, send))
	require.Eventually(t, func() bool { return count() >= 1 }, 10*time.Second, 50*time.Millisecond, "the ticker should fire")
	close(stop)
	wg.Wait()

	require.GreaterOrEqual(t, count(), 2, "one tick plus the final sample")
	for _, r := range replies {
		require.Equal(t, ReplyState, r.Type)
		var st State
		require.NoError(t, json.Unmarshal(r.State, &st))
		require.Positive(t, st.NumCPU)
	}
}

func TestCancelOnSignal(t *testing.T) {
	replies := captureLog(t)

	// Normal termination: stopping the sampler releases the watcher.
	stop := make(chan struct{})
	close(stop)
	CancelOnSignal(func(error) { t.Fatal("cancel must not be called") }, stop)()

	// A signal cancels the run with a cause naming it.
	prev := signalNotify
	signalNotify = func(c chan<- os.Signal, _ ...os.Signal) { c <- syscall.SIGTERM }
	t.Cleanup(func() { signalNotify = prev })

	var cause error
	CancelOnSignal(func(err error) { cause = err }, make(chan struct{}))()
	require.Error(t, cause)
	require.Contains(t, cause.Error(), "terminated")
	require.Len(t, *replies, 1)
	require.Equal(t, ReplyInfo, (*replies)[0].Type)
}

func TestLogLevels(t *testing.T) {
	replies := captureLog(t)
	LogInfo("plain")
	LogWarn("%s %d", "formatted", 1)
	LogError("error")
	require.Equal(t, []ExecReply{
		{Type: ReplyInfo, Message: "plain"},
		{Type: ReplyWarning, Message: "formatted 1"},
		{Type: ReplyError, Message: "error"},
	}, *replies)
}

func TestTerminalErrorSync(t *testing.T) {
	require.NoError(t, terminalError("sync", &Report{Sync: &SyncsReport{}}, State{}))
	err := terminalError("sync", &Report{Sync: &SyncsReport{Errors: 2}}, State{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "2")
}

func TestInventoryWithoutPlugin(t *testing.T) {
	src := &Configuration{Type: "inventory", Integration: Integration{Name: "nope"}}
	err := listInventory(newTestContext(t), &ExecPayload{Op: "inventory", Source: src}, func(*ExecReply) {})
	require.ErrorIs(t, err, plugin.ErrPluginNotExist)
}

// A connector that fails to close is reported as a warning in the stream,
// which is the only way Main's own log sink is exercised.
func TestMainReportsCloseFailures(t *testing.T) {
	loadFakePlugin(t, "importer", integration.ConnectorTypeImporter, "")
	source := fakeConf("source", t.TempDir(), failField("close"))
	code, replies := runMain(t, []string{"-cache", t.TempDir()}, payload(t, &ExecPayload{Op: "test", Source: source}))
	require.Equal(t, 0, code)
	require.Equal(t, ReplySuccess, lastReply(replies).Type)
	var warned bool
	for _, r := range replies {
		if r.Type == ReplyWarning && strings.Contains(r.Message, "close failed") {
			warned = true
		}
	}
	require.True(t, warned, "the close failure must be in the stream: %v", replies)
}
