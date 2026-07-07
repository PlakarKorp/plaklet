package plaklet

import (
	"fmt"
	"time"

	"github.com/PlakarKorp/kloset/kcontext"
	"github.com/PlakarKorp/kloset/locate"
	"github.com/PlakarKorp/kloset/objects"
	"github.com/PlakarKorp/kloset/repository"
	"github.com/PlakarKorp/kloset/snapshot"
	"github.com/PlakarKorp/kloset/snapshot/vfs"
)

// synchronize copies snapshots from a source store to a target store, skipping
// any the target already has. Both source and target are kloset repositories.
func synchronize(ctx *kcontext.KContext, input *ExecPayload) (*Report, error) {
	if input.Source == nil || input.Target == nil {
		return nil, fmt.Errorf("source and target must be set for sync")
	}

	srcStore, srcPass, _, err := mkstorage(ctx, input.Source)
	if err != nil {
		return nil, err
	}
	defer srcStore.Close(ctx)

	srcRepo, err := openrepo(ctx, srcStore, srcPass)
	if err != nil {
		return nil, err
	}

	dstStore, dstPass, _, err := mkstorage(ctx, input.Target)
	if err != nil {
		return nil, err
	}
	defer dstStore.Close(ctx)

	dstRepo, err := openrepo(ctx, dstStore, dstPass)
	if err != nil {
		return nil, err
	}

	// Snapshots already present at the destination are skipped.
	present := make(map[objects.MAC]struct{})
	for id, err := range dstRepo.ListSnapshots() {
		if err != nil {
			return nil, fmt.Errorf("could not list peer snapshots: %w", err)
		}
		present[id] = struct{}{}
	}

	srcIDs, err := locate.LocateSnapshotIDs(srcRepo, locateOptions(input.TaskConfig))
	if err != nil {
		return nil, fmt.Errorf("could not locate source snapshots: %w", err)
	}

	start := time.Now()
	ssr := SyncsReport{}

	for _, id := range srcIDs {
		if _, exists := present[id]; exists {
			continue
		}
		sr, err := syncSnap(srcRepo, dstRepo, id)
		if err != nil {
			ssr.Errors++
			continue
		}
		ssr.LogicalSize += sr.LogicalSize
		ssr.Syncs = append(ssr.Syncs, *sr)
	}

	ssr.Took = time.Since(start)
	return &Report{Type: input.Op, Sync: &ssr}, nil
}

// syncSnap copies one snapshot from src to dst, reusing the destination's latest
// matching snapshot as a VFS parent cache to avoid re-transferring unchanged
// data.
func syncSnap(srcRepo, dstRepo *repository.Repository, id objects.MAC) (*SyncReport, error) {
	srcSnap, err := snapshot.Load(srcRepo, id)
	if err != nil {
		return nil, err
	}
	defer srcSnap.Close()

	dstSnap, err := snapshot.Create(dstRepo, repository.DefaultType, "", srcSnap.Header.Identifier, &snapshot.BuilderOptions{
		// Fold each new state into the destination repo's aggregated state in
		// process (see backup.go for why the edge does this itself).
		StateRefresher: func(_ objects.MAC, _ bool) error {
			return dstRepo.RebuildState()
		},
	})
	if err != nil {
		return nil, err
	}
	defer dstSnap.Close()

	// Preserve the original snapshot metadata rather than the freshly-created one.
	dstSnap.Header = srcSnap.Header

	// Find an existing destination snapshot for the same source to seed the VFS
	// cache, so unchanged files are not re-read.
	var parentVFS *vfs.Filesystem
	imp := srcSnap.Header.GetSource(0).Importer
	parentIDs, _, err := locate.Match(dstRepo, &locate.LocateOptions{
		Filters: locate.LocateFilters{
			Latest:  true,
			Roots:   []string{imp.Directory},
			Types:   []string{imp.Type},
			Origins: []string{imp.Origin},
		},
	})
	if err != nil {
		return nil, err
	}
	if len(parentIDs) != 0 {
		parent, err := snapshot.Load(dstRepo, parentIDs[0])
		if err != nil {
			return nil, err
		}
		defer parent.Close()
		if parentVFS, err = parent.FilesystemWithCache(); err != nil {
			return nil, err
		}
	}
	dstSnap.WithVFSCache(parentVFS)

	start := time.Now()
	if err := srcSnap.Synchronize(dstSnap); err != nil {
		return nil, err
	}

	src := dstSnap.Header.GetSource(0)
	logical := src.Summary.Directory.Size + src.Summary.Below.Size

	return &SyncReport{
		SnapshotID:           dstSnap.Header.Identifier[:],
		SnapshotCreationTime: dstSnap.Header.Timestamp,
		Took:                 time.Since(start),
		Name:                 fmt.Sprintf("%s sync of %s", src.Importer.Type, src.Importer.Origin),
		SourceOrigin:         src.Importer.Origin,
		Root:                 src.Importer.Directory,
		Size:                 int(logical),
		Items:                int(src.Summary.Directory.Files + src.Summary.Below.Files),
		Tags:                 dstSnap.Header.Tags,
		Environment:          dstSnap.Header.Environment,
		Category:             dstSnap.Header.Category,
		Dataset:              dstSnap.Header.Dataset,
		DataClasses:          dstSnap.Header.DataClasses,
		LogicalSize:          logical,
		Content: BackupContent{
			Files:       src.Summary.Directory.Files + src.Summary.Below.Files,
			Directories: src.Summary.Directory.Directories + src.Summary.Below.Directories,
			Symlinks:    src.Summary.Directory.Symlinks + src.Summary.Below.Symlinks,
			Devices:     src.Summary.Directory.Devices + src.Summary.Below.Devices,
			Pipes:       src.Summary.Directory.Pipes + src.Summary.Below.Pipes,
			Sockets:     src.Summary.Directory.Sockets + src.Summary.Below.Sockets,
		},
		Origin: StoreIO{BytesRead: srcRepo.RBytes(), BytesWritten: srcRepo.WBytes()},
		Target: StoreIO{BytesRead: dstRepo.RBytes(), BytesWritten: dstRepo.WBytes()},
	}, nil
}
