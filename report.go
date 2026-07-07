package plaklet

import "time"

// These report shapes mirror the relevant subset of plakman's reporting package.
// plaklet emits them as the raw JSON payload of a ReplyReport; the control plane
// unmarshals them into its own reporting.Report. Keep the JSON tags aligned with
// plakman/reporting.

type StoreIO struct {
	BytesRead    int64 `json:"bytes_read"`
	BytesWritten int64 `json:"bytes_written"`
}

type BackupContent struct {
	Files       uint64 `json:"files"`
	Directories uint64 `json:"directories"`
	Symlinks    uint64 `json:"symlinks"`
	Devices     uint64 `json:"devices"`
	Pipes       uint64 `json:"pipes"`
	Sockets     uint64 `json:"sockets"`
}

type BackupReport struct {
	SnapshotID           []byte        `json:"snapshot_id"`
	SnapshotCreationTime time.Time     `json:"snapshot_creation_time"`
	Took                 time.Duration `json:"took"`
	Name                 string        `json:"name"`
	Origin               string        `json:"origin"`
	Root                 string        `json:"root"`
	Size                 int           `json:"size"`
	Items                int           `json:"items"`
	Tags                 []string      `json:"tags"`
	Environment          string        `json:"environment"`
	Category             string        `json:"category"`
	Dataset              string        `json:"dataset"`
	DataClasses          []string      `json:"data_classes"`
	LogicalSize          uint64        `json:"logical_size"`
	Content              BackupContent `json:"content"`
	Errors               int           `json:"errors"`
	Store                StoreIO       `json:"store"`
}

type CheckReport struct {
	SnapshotID []byte        `json:"snapshot_id"`
	Took       time.Duration `json:"took"`
	Store      StoreIO       `json:"store"`
}

type ChecksReport struct {
	Took        time.Duration `json:"took"`
	LogicalSize uint64        `json:"logical_size"`
	Errors      uint64        `json:"errors"`
	Checks      []CheckReport `json:"checks"`
}

type RestoreReport struct {
	SnapshotID  []byte        `json:"snapshot_id"`
	Took        time.Duration `json:"took"`
	LogicalSize uint64        `json:"logical_size"`
	Content     BackupContent `json:"content"`
	Store       StoreIO       `json:"store"`
}

type SyncIO struct {
	Origin StoreIO `json:"origin"`
	Target StoreIO `json:"target"`
}

type SyncReport struct {
	SnapshotID           []byte        `json:"snapshot_id"`
	SnapshotCreationTime time.Time     `json:"snapshot_creation_time"`
	Took                 time.Duration `json:"took"`
	Name                 string        `json:"name"`
	SourceOrigin         string        `json:"source_origin"`
	Root                 string        `json:"root"`
	Size                 int           `json:"size"`
	Items                int           `json:"items"`
	Tags                 []string      `json:"tags"`
	Environment          string        `json:"environment"`
	Category             string        `json:"category"`
	Dataset              string        `json:"dataset"`
	DataClasses          []string      `json:"data_classes"`
	LogicalSize          uint64        `json:"logical_size"`
	Content              BackupContent `json:"content"`
	Origin               StoreIO       `json:"origin"`
	Target               StoreIO       `json:"target"`
}

type SyncsReport struct {
	Took        time.Duration `json:"took"`
	LogicalSize uint64        `json:"logical_size"`
	Errors      uint64        `json:"errors"`
	Syncs       []SyncReport  `json:"syncs"`
}

// Report is the top-level object carried by a ReplyReport. Exactly one of the
// operation-specific fields is set, matching Type.
type Report struct {
	Type    string         `json:"type"`
	Backup  *BackupReport  `json:"backup,omitempty"`
	Check   *ChecksReport  `json:"check,omitempty"`
	Restore *RestoreReport `json:"restore,omitempty"`
	Sync    *SyncsReport   `json:"sync,omitempty"`
}
