package cached

import (
	"errors"
	"fmt"
	"io"
	"net"
	"path/filepath"
	"slices"
	"time"

	"github.com/PlakarKorp/kloset/kcontext"
	"github.com/PlakarKorp/kloset/objects"
	"github.com/PlakarKorp/plakman"
	"github.com/google/uuid"
	"github.com/vmihailenco/msgpack/v5"
)

type RequestPkt struct {
	PluginName    string
	PluginVersion string

	Secret      []byte
	RepoID      uuid.UUID
	StoreConfig map[string]string

	// Push the request but don't wait for the actual execution.
	FireAndForget bool

	// If empty do a full rebuild otherwise ingest that file from disk.
	StateID objects.MAC
}

type ResponsePkt struct {
	Err      string
	ExitCode int
}

type Client struct {
	conn net.Conn
	enc  *msgpack.Encoder
	dec  *msgpack.Decoder
}

var (
	ErrWrongVersion = errors.New("cached is running with a different version of plakar")
)

func rebuildStateRequest(ctx *kcontext.KContext, req *RequestPkt) (int, error) {
	client, err := newClient(filepath.Join(ctx.CacheDir, "cached.sock"), false)
	if err != nil {
		return 1, err
	}
	defer client.Close() //nolint:errcheck

	go func() {
		<-ctx.Done()
		_ = client.Close()
	}()

	if err := client.enc.Encode(req); err != nil {
		return 1, err
	}

	response := &ResponsePkt{}
	if err := client.dec.Decode(response); err != nil {
		if errors.Is(err, io.EOF) {
			return 0, nil
		}
		if err := ctx.Err(); err != nil {
			return 1, err
		}
		return 1, fmt.Errorf("failed to decode response: %w", err)
	}

	if response.Err != "" {
		return response.ExitCode, fmt.Errorf("%s", response.Err)
	}

	return response.ExitCode, nil
}

func newClient(socketPath string, ignoreVersion bool) (*Client, error) {
	var (
		attempt int
		conn    net.Conn
		err     error
	)

	for {
		conn, err = net.Dial("unix", socketPath)
		if err == nil {
			// connected successfully!
			break
		}

		attempt++
		if attempt > 200 {
			return nil, errors.New("failed to connect to cached")
		}

		time.Sleep(5 * time.Millisecond)
	}

	encoder := msgpack.NewEncoder(conn)
	decoder := msgpack.NewDecoder(conn)

	c := &Client{
		conn: conn,
		enc:  encoder,
		dec:  decoder,
	}

	if err := c.handshake(ignoreVersion); err != nil {
		return nil, err
	}

	return c, nil
}

func (c *Client) handshake(ignoreVersion bool) error {
	ourvers := []byte(plakman.Version)

	if err := c.enc.Encode(ourvers); err != nil {
		return err
	}

	var cachedvers []byte
	if err := c.dec.Decode(&cachedvers); err != nil {
		return err
	}

	if !ignoreVersion && !slices.Equal(ourvers, cachedvers) {
		return fmt.Errorf("%w (%v)", ErrWrongVersion, string(cachedvers))
	}

	return nil
}

func (c *Client) Close() error {
	return c.conn.Close()
}
