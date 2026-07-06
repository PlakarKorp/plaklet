package main

import (
	"fmt"
	"time"

	"github.com/PlakarKorp/kloset/kcontext"
	"github.com/PlakarKorp/kloset/objects"
	"github.com/PlakarKorp/kloset/repository"
	"github.com/PlakarKorp/kloset/snapshot"
)

// backup runs a source -> store backup, mirroring the plakman executor's backup
// flow but built entirely on kloset. Tags/ignores come from the task config.
func backup(ctx *kcontext.KContext, input *ExecPayload) (*Report, error) {
	if input.Source == nil || input.Target == nil {
		return nil, fmt.Errorf("source and target must be set for backup")
	}

	tags := splitList(input.TaskConfig["tags"])
	ignores := splitList(input.TaskConfig["ignore"])

	imp, err := mkimporter(ctx, input.Source)
	if err != nil {
		return nil, err
	}
	defer imp.Close(ctx)

	store, passphrase, _, err := mkstorage(ctx, input.Target)
	if err != nil {
		return nil, err
	}
	defer store.Close(ctx)

	repo, err := openrepo(ctx, store, passphrase)
	if err != nil {
		return nil, err
	}

	opts := &snapshot.BuilderOptions{
		Name:        "plaklet-" + time.Now().String(),
		Tags:        tags,
		Environment: input.Source.Environment,
		Dataset:     input.Source.Id,
		DataClasses: input.Source.DataClasses,
	}

	snap, err := snapshot.Create(repo, repository.DefaultType, "", objects.NilMac, opts)
	if err != nil {
		return nil, err
	}
	defer snap.Close()

	source, err := snapshot.NewSource(repo.AppContext(), imp)
	if err != nil {
		return nil, err
	}
	if err := source.SetExcludes(ignores); err != nil {
		return nil, err
	}

	if err := snap.Backup(source); err != nil {
		return nil, err
	}
	if err := snap.Commit(); err != nil {
		return nil, err
	}

	src := snap.Header.GetSource(0)
	logical := src.Summary.Directory.Size + src.Summary.Below.Size

	br := BackupReport{
		SnapshotID:           snap.Header.Identifier[:],
		SnapshotCreationTime: snap.Header.Timestamp,
		Took:                 snap.Header.Duration,
		Name:                 fmt.Sprintf("%s backup of %s", src.Importer.Type, src.Importer.Origin),
		Origin:               src.Importer.Origin,
		Root:                 src.Importer.Directory,
		Size:                 int(logical),
		Items:                int(src.Summary.Directory.Files + src.Summary.Below.Files),
		Tags:                 snap.Header.Tags,
		Environment:          snap.Header.Environment,
		Category:             snap.Header.Category,
		Dataset:              snap.Header.Dataset,
		DataClasses:          snap.Header.DataClasses,
		LogicalSize:          logical,
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
	if src.Summary.Directory.Errors+src.Summary.Below.Errors != 0 {
		br.Errors = 1
	}

	return &Report{Type: input.Op, Backup: &br}, nil
}
