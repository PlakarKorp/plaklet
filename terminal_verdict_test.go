package plaklet

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTerminalVerdict(t *testing.T) {
	var partialRestore State
	partialRestore.Files.Error = 2
	partialRestore.Symlinks.Error = 1

	tests := []struct {
		name    string
		op      string
		report  *Report
		state   State
		warning string
		err     string
	}{
		{name: "nil report", op: "backup", report: nil},
		{name: "empty report", op: "backup", report: &Report{}},
		{name: "clean backup", op: "backup", report: &Report{Backup: &BackupReport{}}},
		{
			name:    "partial backup warns",
			op:      "backup",
			report:  &Report{Backup: &BackupReport{Errors: 3}},
			warning: "3 file(s) could not be read",
		},
		{name: "clean restore", op: "restore", report: &Report{Restore: &RestoreReport{}}},
		{
			name:   "partial restore fails",
			op:     "restore",
			report: &Report{Restore: &RestoreReport{}},
			state:  partialRestore,
			err:    "3 entries could not be restored",
		},
		{name: "clean sync", op: "sync", report: &Report{Sync: &SyncsReport{}}},
		{
			name:   "partial sync fails",
			op:     "sync",
			report: &Report{Sync: &SyncsReport{Errors: 2}},
			err:    "2 snapshot(s) could not be synchronized",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			warning, err := terminalVerdict(tc.op, tc.report, tc.state)
			require.Equal(t, tc.warning, warning)
			if tc.err == "" {
				require.NoError(t, err)
			} else {
				require.EqualError(t, err, tc.err)
			}
		})
	}

	// The restore verdict folds the per-entry event counts into the report.
	rep := &Report{Restore: &RestoreReport{}}
	_, err := terminalVerdict("restore", rep, partialRestore)
	require.Error(t, err)
	require.Equal(t, uint64(3), rep.Restore.Errors)
}
