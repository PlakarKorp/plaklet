package plaklet

import (
	"context"
	"io"
)

type Executor interface {
	Execute(context.Context, ExecPayload, chan<- ExecReply, io.Writer) error
}
