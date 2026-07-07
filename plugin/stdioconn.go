package plugin

import (
	"net"
	"os"
	"os/exec"
	"time"
)

type ExitConn interface {
	net.Conn
	Exited() <-chan struct{}
	WaitErr() error
}

var stdioaddr = &net.UnixAddr{Name: "stdio", Net: "unix"}

type StdioConn struct {
	stdin  *os.File
	stdout *os.File
	cmd    *exec.Cmd

	exited chan struct{}

	waitErr error
}

func NewStdioConn(stdin, stdout *os.File, cmd *exec.Cmd) ExitConn {
	c := &StdioConn{
		stdin:  stdin,
		stdout: stdout,
		cmd:    cmd,
		exited: make(chan struct{}),
	}
	go func() {
		c.waitErr = cmd.Wait()
		close(c.exited)
	}()
	return c
}

func (c *StdioConn) Exited() <-chan struct{} {
	return c.exited
}

func (c *StdioConn) WaitErr() error {
	return c.waitErr
}

func (c *StdioConn) Read(b []byte) (int, error)  { return c.stdin.Read(b) }
func (c *StdioConn) Write(b []byte) (int, error) { return c.stdout.Write(b) }
func (c *StdioConn) LocalAddr() net.Addr         { return stdioaddr }
func (c *StdioConn) RemoteAddr() net.Addr        { return stdioaddr }

func (c *StdioConn) Close() (ret error) {
	if err := c.stdin.Close(); err != nil {
		ret = err
	}
	if err := c.stdout.Close(); err != nil {
		ret = err
	}
	<-c.exited
	if c.waitErr != nil {
		ret = c.waitErr
	}
	return
}

func (c *StdioConn) SetDeadline(t time.Time) error {
	if err := c.SetReadDeadline(t); err != nil {
		return err
	}
	return c.SetWriteDeadline(t)
}

func (c *StdioConn) SetReadDeadline(t time.Time) error  { return c.stdin.SetReadDeadline(t) }
func (c *StdioConn) SetWriteDeadline(t time.Time) error { return c.stdout.SetWriteDeadline(t) }
