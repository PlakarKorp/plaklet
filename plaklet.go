// Package plaklet is a single-shot task executor built on kloset. It reads one
// ExecPayload as JSON from stdin, runs the requested operation (backup, check,
// ...) against kloset connectors linked in-process, and streams ExecReply
// messages as JSON to stdout — a terminal success/failure reply last.
//
// It depends only on kloset and a set of built-in connectors (see
// connectors.go); it has no dependency on plakman. It runs either as the
// standalone `plaklet` binary (see cmd/plaklet) or embedded in a driver such as
// plakar-edge, which invokes Main directly.
package plaklet

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/PlakarKorp/kloset/caching"
	"github.com/PlakarKorp/kloset/caching/pebble"
	"github.com/PlakarKorp/kloset/kcontext"
	"github.com/PlakarKorp/kloset/logging"
	"github.com/PlakarKorp/pkg"
)

// splitList parses a comma-separated task-config value into a trimmed,
// empty-free slice.
func splitList(v string) []string {
	if v == "" {
		return nil
	}
	var out []string
	for _, p := range strings.Split(v, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// Main runs the plaklet executor with the given argument list (excluding the
// program name) and returns a process exit code. It is the single entry point
// for both the standalone binary and an embedding driver like plakar-edge.
func Main(args []string) int {
	var pkgdir, cachedir string
	var quiet bool
	var cpu, concurrency int

	fs := flag.NewFlagSet("plaklet", flag.ContinueOnError)
	fs.StringVar(&pkgdir, "pkg", "", "package/integrations directory (reserved; connectors are linked in-process)")
	fs.StringVar(&cachedir, "cache", "", "cache directory (required)")
	fs.IntVar(&cpu, "cpu", max(runtime.GOMAXPROCS(0)-1, 1), "number of CPUs to use")
	fs.IntVar(&concurrency, "concurrency", 0, "maximum concurrency (0 = default)")
	fs.BoolVar(&quiet, "quiet", false, "quiet")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	if cachedir == "" {
		fmt.Fprintln(os.Stderr, "plaklet: -cache is required")
		return 2
	}
	if cpu > 0 {
		runtime.GOMAXPROCS(cpu)
	}

	if err := os.MkdirAll(cachedir, 0o700); err != nil {
		fmt.Fprintf(os.Stderr, "plaklet: cache dir: %s\n", err)
		return 1
	}

	ctx := kcontext.NewKContext()
	ctx.CacheDir = cachedir
	// MaxConcurrency drives the number of scan/backup worker goroutines. It
	// defaults to 0 in kloset, which starts *no* workers and deadlocks the
	// importer, so always give it a sane value.
	if concurrency <= 0 {
		concurrency = max(runtime.NumCPU(), 1)
	}
	ctx.MaxConcurrency = concurrency
	ctx.SetLogger(logging.NewLogger(os.Stderr, os.Stderr))
	ctx.SetCache(caching.NewManager(pebble.Constructor(cachedir)))
	defer ctx.GetCache().Close()

	// Load any connector packages (.ptar plugins) present in pkgdir. Compiled-in
	// connectors (fs, stdio, tar, ...) work without this; plugins (s3, sftp, ...)
	// the edge fetched through the control-plane proxy are registered here.
	if pkgdir != "" {
		backend, err := pkg.NewFlatBackend(ctx, pkgdir, cachedir, &pkg.FlatBackendOptions{
			LoadHook:   pkgLoadHook,
			UnloadHook: pkgUnloadHook,
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "plaklet: package backend: %s\n", err)
			return 1
		}
		if err := backend.LoadAll(); err != nil {
			fmt.Fprintf(os.Stderr, "plaklet: load packages: %s\n", err)
			return 1
		}
	}

	enc := json.NewEncoder(os.Stdout)
	var sendMu sync.Mutex
	send := func(r *ExecReply) {
		sendMu.Lock()
		_ = enc.Encode(r)
		sendMu.Unlock()
	}

	var input ExecPayload
	if err := json.NewDecoder(os.Stdin).Decode(&input); err != nil {
		if errors.Is(err, io.EOF) {
			return 0
		}
		fmt.Fprintf(os.Stderr, "plaklet: decode payload: %s\n", err)
		return 1
	}

	// The event listener drains kloset's event bus (also required to keep the
	// importer from blocking on a full channel) and folds events into a live
	// State.
	listener := newEventListener()
	listener.Run(ctx.Events())

	// Sample CPU/memory and read/write throughput on a ticker and stream them as
	// ReplyState, so the control plane gets a live resource/throughput graph.
	stateStop := make(chan struct{})
	stateDone := make(chan struct{})
	go func() {
		defer close(stateDone)
		resources := newResourceSampler()
		network := newNetworkSampler(input.Op)
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()

		emit := func() {
			resources.sample()
			st := listener.State()
			network.sample(st.IO)
			st.Resources = resources.snapshot()
			st.NumCPU = resources.numCPU()
			st.Network = network.snapshot()
			if raw, err := json.Marshal(&st); err == nil {
				send(&ExecReply{Type: ReplyState, State: raw})
			}
		}

		for {
			select {
			case <-stateStop:
				emit() // final sample
				return
			case <-ticker.C:
				emit()
			}
		}
	}()

	report, err := dispatch(ctx, &input)

	// Stop sampling, then close and drain the event bus.
	close(stateStop)
	<-stateDone
	ctx.Events().Close()
	listener.Wait()

	// listener.Wait() has drained every event, so the final State is complete
	// and race-free here — safe to fold event-only errors into the verdict.
	if err == nil {
		err = terminalError(input.Op, report, listener.State())
	}

	if err != nil {
		send(&ExecReply{Type: ReplyFailure, Message: fmt.Sprintf("%s failed: %s", input.Op, err)})
		return 0
	}

	if report != nil {
		if raw, merrr := json.Marshal(report); merrr == nil {
			send(&ExecReply{Type: ReplyReport, Report: raw})
		}
	}
	send(&ExecReply{Type: ReplySuccess})
	return 0
}

// terminalError decides whether a completed operation should fail the job, for
// the operations whose failures surface only after the fact rather than as a
// returned error. A backup or restore commits/returns nil even when it couldn't
// read or write some entries; check tallies snapshot errors in its own report
// and already returns an error itself. We treat any such error count as a job
// failure — a partial run is not a clean success. state carries the per-entry
// error counters folded from the (now fully drained) event bus, used for
// restore whose failures live only in events. Mutates report to record the
// count. Returns nil when the run is clean.
func terminalError(op string, report *Report, state State) error {
	if report == nil {
		return nil
	}
	switch op {
	case "backup":
		if report.Backup != nil && report.Backup.Errors > 0 {
			return fmt.Errorf("backup failed: %d file(s) could not be read", report.Backup.Errors)
		}
	case "restore":
		if report.Restore != nil {
			entryErrors := state.Paths.Error + state.Files.Error + state.Dirs.Error +
				state.Symlinks.Error + state.Xattrs.Error
			if entryErrors != 0 {
				report.Restore.Errors = entryErrors
				return fmt.Errorf("restore failed: %d entries could not be restored", entryErrors)
			}
		}
	}
	return nil
}

// dispatch routes an operation to its handler. Only the operations a remote edge
// realistically runs are implemented; others return an explicit error.
func dispatch(ctx *kcontext.KContext, input *ExecPayload) (*Report, error) {
	switch input.Op {
	case "backup":
		return backup(ctx, input)
	case "check":
		return check(ctx, input)
	case "restore":
		return restore(ctx, input)
	case "sync":
		return synchronize(ctx, input)
	case "create":
		return create(ctx, input)
	default:
		return nil, fmt.Errorf("unsupported operation %q", input.Op)
	}
}
