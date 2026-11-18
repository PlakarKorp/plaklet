/*
 * Copyright (c) 2025 Mathieu Masson <mathieu@plakar.io>
 *
 * Permission to use, copy, modify, and distribute this software for any
 * purpose with or without fee is hereby granted, provided that the above
 * copyright notice and this permission notice appear in all copies.
 *
 * THE SOFTWARE IS PROVIDED "AS IS" AND THE AUTHOR DISCLAIMS ALL WARRANTIES
 * WITH REGARD TO THIS SOFTWARE INCLUDING ALL IMPLIED WARRANTIES OF
 * MERCHANTABILITY AND FITNESS. IN NO EVENT SHALL THE AUTHOR BE LIABLE FOR
 * ANY SPECIAL, DIRECT, INDIRECT, OR CONSEQUENTIAL DAMAGES OR ANY DAMAGES
 * WHATSOEVER RESULTING FROM LOSS OF USE, DATA OR PROFITS, WHETHER IN AN
 * ACTION OF CONTRACT, NEGLIGENCE OR OTHER TORTIOUS ACTION, ARISING OUT OF
 * OR IN CONNECTION WITH THE USE OR PERFORMANCE OF THIS SOFTWARE.
 */

package cached

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/PlakarKorp/integration-sdk"
	"github.com/PlakarKorp/kloset/caching"
	"github.com/PlakarKorp/kloset/caching/pebble"
	"github.com/PlakarKorp/kloset/connectors/storage"
	"github.com/PlakarKorp/kloset/encryption"
	"github.com/PlakarKorp/kloset/kcontext"
	klogging "github.com/PlakarKorp/kloset/logging"
	"github.com/PlakarKorp/kloset/repository"
	"github.com/PlakarKorp/pkg"
	"github.com/PlakarKorp/plaklet/logging"
	"github.com/PlakarKorp/plaklet/plugin"
	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/vmihailenco/msgpack/v5"
)

const (
	jobQueueIdleDelay = 1 * time.Minute
)

type Cached struct {
	pkgDir     string
	cacheDir   string
	socketPath string
	registry   *plugin.Registry
	listener   net.Listener

	jobMtx   sync.Mutex
	jobQueue map[uuid.UUID](chan jobReq)
}

type jobReq struct {
	stateID integration.MAC
	ch      chan error
}

func NewCached(cachedir string, pkgdir string) *Cached {
	var cmd Cached
	cmd.cacheDir = cachedir
	cmd.pkgDir = pkgdir
	cmd.socketPath = filepath.Join(cachedir, "cached.sock")
	cmd.jobMtx = sync.Mutex{}
	cmd.jobQueue = make(map[uuid.UUID]chan jobReq)
	return &cmd
}

func RunCached(cachedir string, pkgdir string) error {
	ktx := kcontext.NewKContext()
	ktx.MaxConcurrency = 1
	ktx.CacheDir = cachedir
	ktx.SetCache(caching.NewManager(pebble.Constructor(cachedir)))
	ktx.SetLogger(klogging.NewLogger(os.Stdout, os.Stderr))
	return NewCached(cachedir, pkgdir).ListenAndServe(ktx)
}

func (cmd *Cached) reloadPlugins(ctx *kcontext.KContext) error {
	registry := plugin.NewRegistry()
	backend, err := pkg.NewFlatBackend(ctx, cmd.pkgDir, cmd.cacheDir, &pkg.FlatBackendOptions{
		LoadHook: func(m *pkg.Manifest, p *pkg.Package, pkgdir string) {
			if err := registry.RegisterPlugins(m, p, pkgdir); err != nil {
				_ = err
			}
		},
		UnloadHook: registry.UnregisterPlugins,
	})
	if err != nil {
		return err
	}
	if err := backend.LoadAll(); err != nil {
		return err
	}

	cmd.registry = registry
	return nil
}

func (cmd *Cached) Close() error {
	if cmd.listener != nil {
		_ = cmd.listener.Close()
	}
	if err := os.Remove(cmd.socketPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (cmd *Cached) ListenAndServe(ctx *kcontext.KContext) error {
	lock, err := LockedFile(cmd.socketPath + ".cached-lock")
	if err != nil {
		return errors.New("failed to obtain lock")
	}
	conn, err := net.Dial("unix", cmd.socketPath)
	if err == nil {
		lock.Unlock()
		_ = conn.Close()
		return errors.New("cached already running")
	}
	_ = os.Remove(cmd.socketPath)

	listener, err := net.Listen("unix", cmd.socketPath)
	lock.Unlock()

	if err != nil {
		return fmt.Errorf("failed to bind the socket: %w", err)
	}

	if err := cmd.reloadPlugins(ctx); err != nil {
		return fmt.Errorf("failed to load plugins: %w", err)
	}

	cancelled := false
	go func() {
		<-ctx.Done()
		cancelled = true
		_ = listener.Close()
	}()

	for {
		conn, err := listener.Accept()
		if err != nil {
			if cancelled {
				return ctx.Err()
			}

			opErr := &net.OpError{}
			if errors.As(err, &opErr) && opErr.Err.Error() == "use of closed network connection" {
				return nil
			}
			// TODO: we should retry / wait and retry on
			// some errors, not everything is fatal.
			return err
		}

		go cmd.handleCachedClient(ctx, conn)
	}
}

func (cmd *Cached) handleCachedClient(ctx *kcontext.KContext, conn net.Conn) {
	defer conn.Close() //nolint:errcheck

	encoder := msgpack.NewEncoder(conn)
	decoder := msgpack.NewDecoder(conn)

	// handshake
	var (
		clientvers []byte
		ourvers    = []byte(Version)
	)
	if err := decoder.Decode(&clientvers); err != nil {
		return
	}
	if err := encoder.Encode(ourvers); err != nil {
		return
	}

	pkt := &RequestPkt{}
	if err := decoder.Decode(pkt); err != nil {
		if isDisconnectError(err) {
			logging.Warn("client disconnected during initial request")
			return
		}
		logging.Warn("Failed to decode RPC: %v", err)
		return
	}

	logging.Info("cached: rebuild request for %s", pkt.RepoID)

	// Is there already a job goroutine running for this repo:
	var jq chan jobReq
	var ok bool
	var err error
	cmd.jobMtx.Lock()
	if jq, ok = cmd.jobQueue[pkt.RepoID]; !ok {
		jq = make(chan jobReq, 1024)
		err = cmd.rebuildJob(ctx, jq, pkt)

		if err == nil {
			cmd.jobQueue[pkt.RepoID] = jq
		}
	}
	cmd.jobMtx.Unlock()

	if err == nil {
		j := jobReq{
			stateID: pkt.StateID,
		}

		if !pkt.FireAndForget {
			j.ch = make(chan error, 1)
		}

		jq <- j

		if !pkt.FireAndForget {
			err = <-j.ch
		}
	}

	errStr := ""
	exitCode := 0
	if err != nil {
		log.Printf("cached: request failed for %s: %v", pkt.RepoID, err)
		errStr = err.Error()
		exitCode = -1
	}

	response := &ResponsePkt{
		ExitCode: exitCode,
		Err:      errStr,
	}
	if err := encoder.Encode(&response); err != nil {
		logging.Warn("client write error: %v", err)
	}
}

func (cmd *Cached) mkstore(ktx *kcontext.KContext, pkgname, pkgver string, params map[string]string) (context.Context, integration.Store, error) {
	location := params["location"]
	proto, _, _ := strings.Cut(location, "://")

	// Use builtin integrations right away.
	// This is necessary for fs because it is also packaged but the packaged
	// version is just a placeholder and will always fail.
	_, _, newStorage, _, found := integration.LookupStore(proto)
	if found {
		ctx := context.Background()
		store, err := newStorage(ctx, proto, params)
		if err != nil {
			return nil, nil, err
		}
		return ctx, store, nil
	}

	p, err := cmd.registry.GetPlugin(pkgname, pkgver, integration.ConnectorTypeStorage, proto)
	if err != nil {
		if errors.Is(err, plugin.ErrPluginNotExist) {
			if err2 := cmd.reloadPlugins(ktx); err2 != nil {
				logging.Warn("cached: failed to reload plugins: %v", err2)
				return nil, nil, err
			}
			p, err = cmd.registry.GetPlugin(pkgname, pkgver, integration.ConnectorTypeStorage, proto)
		}
		if err != nil {
			return nil, nil, err
		}
	}

	ctx := context.Background()
	store, err := p.NewStorage(ctx, proto, params)
	if err != nil {
		return nil, nil, err
	}

	return ctx, store, nil
}

func (cmd *Cached) rebuildJob(ctx *kcontext.KContext, jobChan chan jobReq, req *RequestPkt) error {
	_, store, err := cmd.mkstore(ctx, req.PluginName, req.PluginVersion, req.StoreConfig)
	if err != nil {
		return fmt.Errorf("failed to run storage: %w", err)
	}

	serializedConfig, err := store.Open(ctx)
	if err != nil {
		_ = store.Close(ctx)
		return fmt.Errorf("failed to open storage: %w", err)
	}

	key, err := checkPassphrase(req.Secret, serializedConfig)
	if err != nil {
		_ = store.Close(ctx)
		return fmt.Errorf("incorrect passphrase %s: %w", string(req.Secret), err)
	}

	repo, err := repository.NewRepository(ctx, key, store, serializedConfig, &repository.RepositoryOpts{RWStateCache: true})
	if err != nil {
		_ = store.Close(ctx)
		return fmt.Errorf("failed to open repository: %w", err)
	}

	if req.RepoID != repo.Configuration().RepositoryID {
		err := fmt.Errorf("invalid uuid given %q repository id is %q", req.RepoID.String(),
			repo.Configuration().RepositoryID.String())
		_ = repo.Close()
		_ = store.Close(ctx)
		return err
	}

	go func() {
		defer store.Close(ctx) //nolint:errcheck
		defer repo.Close()     //nolint:errcheck

		repoID := repo.Configuration().RepositoryID

		log.Printf("cached: %s: starting job queue", repoID)
		defer log.Printf("cached: %s: exiting job queue", repoID)

		teardown := func(reason string) {
			log.Printf("cached: %s: tearing down job queue: %s", repoID, reason)
			cmd.jobMtx.Lock()
			delete(cmd.jobQueue, repoID)
			cmd.jobMtx.Unlock()
			close(jobChan)
			for job := range jobChan {
				if job.ch != nil {
					job.ch <- fmt.Errorf("cached: job queue torn down: %s", reason)
					close(job.ch)
				}
			}
		}

		for {
			select {
			case job := <-jobChan:
				var err error
				if job.stateID == integration.NilMac {
					log.Printf("cached: %s: rebuild state", repoID)
					err = repo.RebuildState()
				} else {
					log.Printf("cached: %s: ingest state file %s", repoID, job.stateID.FormatHex())
					err = repo.IngestStateFile(job.stateID)
				}

				if err != nil {
					log.Printf("cached: %s: job failed: %v", repoID, err)
				} else {
					log.Printf("cached: %s: job done", repoID)
				}

				if job.ch != nil {
					job.ch <- err
					close(job.ch)
				}

				if isTransportDead(err) {
					teardown(fmt.Sprintf("transport error on job: %v", err))
					return
				}

			case <-time.After(jobQueueIdleDelay):
				teardown("idle out")
				return

			case <-ctx.Done():
				teardown("context canceled")
				return
			}
		}
	}()

	return nil
}

func checkPassphrase(secret []byte, storageConfig []byte) ([]byte, error) {
	config, err := storage.NewConfigurationFromWrappedBytes(storageConfig)
	if err != nil {
		return nil, err
	}

	if config.Encryption == nil {
		return nil, nil
	}

	key, err := encryption.DeriveKey(config.Encryption.KDFParams, secret)
	if err != nil {
		return nil, err
	}

	if !encryption.VerifyCanary(config.Encryption, key) {
		return nil, errors.New("failed to verify key")
	}

	return key, nil
}

func isTransportDead(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return true
	}
	return status.Code(err) == codes.Unavailable
}

func isDisconnectError(err error) bool {
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return true
	}
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}
