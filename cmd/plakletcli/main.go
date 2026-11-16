package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"

	"github.com/PlakarKorp/plaklet"
)

func main() {
	var c plaklet.CommandExecutor
	if len(os.Args) < 2 {
		panic(fmt.Errorf("usage: %s <executable> [args...]", os.Args[0]))
	}

	c.Bin = os.Args[1]
	c.Args = os.Args[2:]
	c.Env = os.Environ()

	payload, err := readPayload(os.Stdin)
	if err != nil {
		panic(err)
	}
	ctx := context.Background()
	replies := make(chan plaklet.ExecReply)

	var wg sync.WaitGroup
	wg.Go(func() {
		for reply := range replies {
			fmt.Println(reply)
		}
	})

	err = c.Execute(ctx, *payload, replies, os.Stderr)
	wg.Wait()

	if err != nil {
		panic(err)
	}
}

func readPayload(rd io.Reader) (*plaklet.ExecPayload, error) {
	var r plaklet.ExecPayload
	err := json.NewDecoder(rd).Decode(&r)
	if err != nil {
		return nil, err
	}
	return &r, nil
}
