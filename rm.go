package plaklet

import (
	"encoding/hex"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/PlakarKorp/kloset/kcontext"
	"github.com/PlakarKorp/kloset/objects"
)

// parseSnapshotIDs decodes the full hex snapshot identifiers an rm task names
// into MACs. Every identifier must be complete (no prefixes): the control plane
// resolved them before dispatch, so a short or malformed one is a bug upstream,
// not something to guess about against the store.
func parseSnapshotIDs(ids []string) ([]objects.MAC, error) {
	macs := make([]objects.MAC, 0, len(ids))
	for _, id := range ids {
		raw, err := hex.DecodeString(id)
		if err != nil {
			return nil, fmt.Errorf("invalid snapshot id %q: %w", id, err)
		}
		if len(raw) != len(objects.MAC{}) {
			return nil, fmt.Errorf("invalid snapshot id %q: expected %d bytes", id, len(objects.MAC{}))
		}
		var mac objects.MAC
		copy(mac[:], raw)
		macs = append(macs, mac)
	}
	return macs, nil
}

// rm removes the snapshots named by the task config from the source store,
// mirroring the plakman plaklet's rm op. The control plane turns a prune's
// retention decision into an rm, so this is what retention runs on an edge.
func rm(ctx *kcontext.KContext, input *ExecPayload) (*Report, error) {
	if input.Source == nil {
		return nil, fmt.Errorf("source must be set for rm")
	}

	snapshotIDs, err := parseSnapshotIDs(indexedList(input.TaskConfig, "snapshot_ids"))
	if err != nil {
		return nil, err
	}
	// An empty list is a run with nothing to do, not a failure: the control
	// plane names what an rm removes, and a retention decision that matched
	// nothing names nothing. Answer before the store is even opened.
	if len(snapshotIDs) == 0 {
		return &Report{Type: "rm", Rm: &RmReport{}}, nil
	}

	store, passphrase, _, err := mkstorage(ctx, input.Source)
	if err != nil {
		return nil, err
	}
	defer store.Close(ctx)

	repo, err := openrepo(ctx, store, passphrase)
	if err != nil {
		return nil, err
	}

	rmq := make(chan objects.MAC, ctx.MaxConcurrency)
	removed := make(chan []byte)
	rp := RmReport{}

	var errors atomic.Uint64

	var reportWG sync.WaitGroup
	reportWG.Go(func() {
		for snapshotID := range removed {
			rp.SnapshotIDs = append(rp.SnapshotIDs, snapshotID)
		}
	})

	var wg sync.WaitGroup
	for range ctx.MaxConcurrency {
		wg.Go(func() {
			for snapshotID := range rmq {
				if err := repo.DeleteSnapshot(snapshotID); err != nil {
					ctx.GetLogger().Warn("rm: failed to remove snapshot %x: %s", snapshotID[:4], err)
					errors.Add(1)
				} else {
					removed <- snapshotID[:]
				}
			}
		})
	}

	for _, snapshotID := range snapshotIDs {
		rmq <- snapshotID
	}
	close(rmq)

	wg.Wait()
	close(removed)
	reportWG.Wait()

	// Partial failures are carried in the report, not as a job failure — same
	// contract as the plakman plaklet: the snapshots that could be removed are.
	rp.Errors = errors.Load()
	return &Report{Type: "rm", Rm: &rp}, nil
}
