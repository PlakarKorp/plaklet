package plaklet

import (
	"fmt"
	"path"
	"strings"

	"github.com/PlakarKorp/kloset/kcontext"
	"github.com/PlakarKorp/kloset/locate"
	"github.com/PlakarKorp/kloset/snapshot"
)

// restore exports a single snapshot from a store (source) to a destination
// (target exporter). It selects exactly one snapshot: an explicit "snapshot" id
// in the task config, otherwise the latest match.
func restore(ctx *kcontext.KContext, input *ExecPayload) (*Report, error) {
	if input.Source == nil || input.Target == nil {
		return nil, fmt.Errorf("source and target must be set for restore")
	}

	exp, err := mkexporter(ctx, input.Target)
	if err != nil {
		return nil, err
	}
	defer exp.Close(ctx)

	store, passphrase, _, err := mkstorage(ctx, input.Source)
	if err != nil {
		return nil, err
	}
	defer store.Close(ctx)

	repo, err := openrepo(ctx, store, passphrase)
	if err != nil {
		return nil, err
	}

	lo := locateOptions(input.TaskConfig)
	if len(lo.Filters.IDs) != 1 {
		lo.Filters.Latest = true
	}

	snapshotIDs, err := locate.LocateSnapshotIDs(repo, lo)
	if err != nil {
		return nil, fmt.Errorf("could not fetch snapshots list: %w", err)
	}
	if len(snapshotIDs) == 0 {
		return nil, fmt.Errorf("no snapshots found")
	}
	if len(snapshotIDs) > 1 {
		return nil, fmt.Errorf("multiple snapshots found, please specify one")
	}

	opts := &snapshot.ExportOptions{}

	snap, pathname, relative, err := locate.OpenSnapshotByPathRelative(repo, fmt.Sprintf("%x:", snapshotIDs[0]))
	if err != nil {
		return nil, err
	}
	defer snap.Close()

	if relative != "" {
		if !strings.HasSuffix(relative, "/") {
			opts.Strip = path.Dir(pathname)
		} else {
			opts.Strip = pathname
		}
	}

	if err := snap.Export(exp, pathname, opts); err != nil {
		return nil, err
	}

	src := snap.Header.GetSource(0)
	logical := src.Summary.Directory.Size + src.Summary.Below.Size

	rr := RestoreReport{
		SnapshotID:  snap.Header.Identifier[:],
		Took:        snap.Header.Duration,
		LogicalSize: logical,
		Content: BackupContent{
			Files:       src.Summary.Directory.Files + src.Summary.Below.Files,
			Directories: src.Summary.Directory.Directories + src.Summary.Below.Directories,
			Symlinks:    src.Summary.Directory.Symlinks + src.Summary.Below.Symlinks,
			Devices:     src.Summary.Directory.Devices + src.Summary.Below.Devices,
			Pipes:       src.Summary.Directory.Pipes + src.Summary.Below.Pipes,
			Sockets:     src.Summary.Directory.Sockets + src.Summary.Below.Sockets,
		},
		Store: StoreIO{
			BytesRead:    repo.RBytes(),
			BytesWritten: repo.WBytes(),
		},
	}

	return &Report{Type: input.Op, Restore: &rr}, nil
}
