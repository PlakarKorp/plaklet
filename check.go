package main

import (
	"fmt"
	"time"

	"github.com/PlakarKorp/kloset/kcontext"
	"github.com/PlakarKorp/kloset/locate"
	"github.com/PlakarKorp/kloset/snapshot"
)

// check verifies the snapshots in a store, mirroring the plakman executor's
// check flow on top of kloset. It checks every snapshot the store holds.
func check(ctx *kcontext.KContext, input *ExecPayload) (*Report, error) {
	if input.Source == nil {
		return nil, fmt.Errorf("source must be set for check")
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

	snapshotIDs, err := locate.LocateSnapshotIDs(repo, locate.NewDefaultLocateOptions())
	if err != nil {
		return nil, err
	}

	checkCache, err := ctx.GetCache().Check()
	if err != nil {
		return nil, err
	}
	defer checkCache.Close()

	start := time.Now()
	ccr := ChecksReport{}

	for _, id := range snapshotIDs {
		path := fmt.Sprintf("%x:", id)

		snap, pathname, err := locate.OpenSnapshotByPath(repo, path)
		if err != nil {
			return &Report{Type: input.Op, Check: &ccr}, err
		}
		snap.SetCheckCache(checkCache)

		snapStart := time.Now()
		rbefore, wbefore := repo.RBytes(), repo.WBytes()

		if err := snap.Check(pathname, &snapshot.CheckOptions{}); err != nil {
			ccr.Errors++
		}

		ccr.Checks = append(ccr.Checks, CheckReport{
			SnapshotID: snap.Header.Identifier[:],
			Took:       time.Since(snapStart),
			Store: StoreIO{
				BytesRead:    repo.RBytes() - rbefore,
				BytesWritten: repo.WBytes() - wbefore,
			},
		})
		snap.Close()
	}

	ccr.Took = time.Since(start)
	return &Report{Type: input.Op, Check: &ccr}, nil
}
