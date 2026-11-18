package plaklet

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/PlakarKorp/kloset/kcontext"
)

type LocalExecutor struct {
	ktx *kcontext.KContext
}

func (e *LocalExecutor) Execute(ctx context.Context, payload ExecPayload, replies chan<- ExecReply, stderr io.Writer) error {
	defer close(replies)
	report, err := e.dispatch(ctx, payload, replies, stderr)

	if report != nil {
		raw, err := json.Marshal(report)
		if err != nil {
			fmt.Fprintf(stderr, "failed to marshal report: %s\n", err)
		} else {
			replies <- ExecReply{Type: ReplyReport, Report: raw}
		}
	}

	if err != nil {
		return err
	}
	return nil
}

func (e *LocalExecutor) dispatch(ctx context.Context, payload ExecPayload, replies chan<- ExecReply, stderr io.Writer) (*Report, error) {
	send := func(reply *ExecReply) {
		replies <- *reply
	}

	switch payload.Op {
	case "backup":
		return backup(e.ktx, &payload)
	case "check":
		return check(e.ktx, &payload)
	case "create":
		return create(e.ktx, &payload)
	case "maintenance":
		return maintenance(e.ktx, &payload)
	case "prune":
		return prune(e.ktx, &payload)
	case "restore":
		return restore(e.ktx, &payload)
	case "rm":
		return rm(e.ktx, &payload)
	case "sync":
		return synchronize(e.ktx, &payload)
	case "test":
		return test(e.ktx, &payload)
	case "inventory":
		return nil, listInventory(e.ktx, &payload, send)
	default:
		return nil, fmt.Errorf("unsupported operation %q", payload.Op)
	}
}
