package plaklet

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"
)

type CommandExecutor struct {
	Name             string
	Bin              string
	Args             []string
	Env              []string
	TerminateTimeout time.Duration
}

var ErrMissingPlakletBin = errors.New("plaklet bin not specified")
var ErrMissingPkgDir = errors.New("missing pkgdir")
var ErrMissingCacheDir = errors.New("missing cachedir")

func (c *CommandExecutor) AddArgs(args ...string) {
	c.Args = append(c.Args, args...)
}

func (c *CommandExecutor) AddEnv(envs ...string) {
	c.Env = append(c.Env, envs...)
}

func (c *CommandExecutor) Execute(ctx context.Context, payload ExecPayload, replies chan<- ExecReply, stderr io.Writer) error {
	defer close(replies)
	bin := c.Bin
	args := c.Args

	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Stderr = stderr
	cmd.Env = c.Env
	cmd.Cancel = func() error {
		return cmd.Process.Signal(syscall.SIGTERM)
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		stdin.Close()
		return err
	}

	if err := cmd.Start(); err != nil {
		stdin.Close()
		stdout.Close()
		return fmt.Errorf("failed to start plaklet: %w", err)
	}

	procName := c.Name
	if procName == "" {
		procName = fmt.Sprintf("%s[%d]", filepath.Base(bin), cmd.Process.Pid)
	}

	if err := json.NewEncoder(stdin).Encode(payload); err != nil {
		stdin.Close()
		_ = cmd.Wait()
		return fmt.Errorf("failed to send payload to plaklet: %w", err)
	}
	stdin.Close()

	procAwaited := make(chan struct{})
	go func() {
		select {
		case <-procAwaited:
			// Process exited normally, just return.
			return
		case <-ctx.Done():
			// Cancelled requested, force kill after a grace delay.
		}

		select {
		case <-procAwaited:
			// Process terminated correctly, just return.
			return
		case <-time.After(c.TerminateTimeout):
			log.Printf("%s: grace period expired, killing process", procName)
			err := cmd.Process.Kill()
			if err != nil {
				log.Printf("%s: kill failed: %s", procName, err)
			}
		}
	}()

	dec := json.NewDecoder(stdout)
	for {
		var r ExecReply
		if err := dec.Decode(&r); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			log.Printf("%s: decode plaklet reply: %v", procName, err)
			if err := stdout.Close(); err != nil {
				log.Printf("%s: failed to close stdout: %v", procName, err)
			}
			break
		}
		replies <- r
	}

	err = cmd.Wait()
	close(procAwaited)
	return err
}
