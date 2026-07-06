// Command plaklet is a single-shot task executor built on kloset. It reads one
// ExecPayload as JSON from stdin, runs the requested operation (backup, check,
// ...) against kloset connectors linked in-process, and streams ExecReply
// messages as JSON to stdout — a terminal success/failure reply last.
//
// It depends only on kloset and a set of built-in connectors (see
// connectors.go); it has no dependency on plakman. It is spawned by a driver
// such as the plakman executor or plakar-edge.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"

	"github.com/PlakarKorp/kloset/caching"
	"github.com/PlakarKorp/kloset/caching/pebble"
	"github.com/PlakarKorp/kloset/kcontext"
	"github.com/PlakarKorp/kloset/logging"
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

func main() {
	os.Exit(run())
}

func run() int {
	var pkgdir, cachedir string
	var quiet bool
	var cpu, concurrency int

	flag.StringVar(&pkgdir, "pkg", "", "package/integrations directory (reserved; connectors are linked in-process)")
	flag.StringVar(&cachedir, "cache", "", "cache directory (required)")
	flag.IntVar(&cpu, "cpu", max(runtime.GOMAXPROCS(0)-1, 1), "number of CPUs to use")
	flag.IntVar(&concurrency, "concurrency", 0, "maximum concurrency (0 = default)")
	flag.BoolVar(&quiet, "quiet", false, "quiet")
	flag.Parse()

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

	// kloset publishes progress onto the context's event bus during backup/check
	// (the fs importer, for one, blocks sending scan events once the buffer
	// fills). Nothing here renders them, but they must be drained or the
	// operation deadlocks. Consume and discard for the lifetime of the run.
	events := ctx.Events()
	eventsDone := make(chan struct{})
	go func() {
		for range events.Listen() {
		}
		close(eventsDone)
	}()

	enc := json.NewEncoder(os.Stdout)
	send := func(r *ExecReply) { _ = enc.Encode(r) }

	var input ExecPayload
	if err := json.NewDecoder(os.Stdin).Decode(&input); err != nil {
		if errors.Is(err, io.EOF) {
			return 0
		}
		fmt.Fprintf(os.Stderr, "plaklet: decode payload: %s\n", err)
		return 1
	}

	report, err := dispatch(ctx, &input)

	// Stop and drain the event bus before emitting the terminal reply.
	events.Close()
	<-eventsDone

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
	default:
		return nil, fmt.Errorf("unsupported operation %q", input.Op)
	}
}
