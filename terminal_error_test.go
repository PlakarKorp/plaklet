package plaklet

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTerminalError(t *testing.T) {
	// A clean backup/restore is not a failure.
	require.NoError(t, terminalError("backup", &Report{Backup: &BackupReport{Errors: 0}}, State{}))
	require.NoError(t, terminalError("restore", &Report{Restore: &RestoreReport{}}, State{}))

	// Nil report / nil section are safe no-ops.
	require.NoError(t, terminalError("backup", nil, State{}))
	require.NoError(t, terminalError("backup", &Report{}, State{}))
	require.NoError(t, terminalError("restore", &Report{}, State{}))

	// A backup that couldn't read some files fails the job (same semantics as
	// restore and check — a partial run is not a clean success).
	err := terminalError("backup", &Report{Backup: &BackupReport{Errors: 3}}, State{})
	require.Error(t, err)
	require.True(t, strings.Contains(err.Error(), "3"), "message should carry the error count: %q", err)

	// A restore whose entries failed to export fails the job; the per-entry
	// counts come from the drained event State, and are folded into the report.
	var st State
	st.Files.Error = 2
	st.Symlinks.Error = 1
	rep := &Report{Restore: &RestoreReport{}}
	err = terminalError("restore", rep, st)
	require.Error(t, err)
	require.Equal(t, uint64(3), rep.Restore.Errors)
	require.True(t, strings.Contains(err.Error(), "3"), "message should carry the error count: %q", err)

	// Operations without a post-hoc failure notion (e.g. sync) are unaffected.
	require.NoError(t, terminalError("sync", &Report{}, State{}))
}
