package plaklet

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBackupWarning(t *testing.T) {
	// A clean backup produces no warning.
	require.Nil(t, backupWarning("backup", &Report{Backup: &BackupReport{Errors: 0}}))

	// Nil report / nil backup section are safe no-ops.
	require.Nil(t, backupWarning("backup", nil))
	require.Nil(t, backupWarning("backup", &Report{}))

	// A backup with per-file errors warns but does not fail — the caller still
	// emits ReplySuccess. The message carries the actual error count.
	w := backupWarning("backup", &Report{Backup: &BackupReport{Errors: 3}})
	require.NotNil(t, w)
	require.Equal(t, ReplyWarning, w.Type)
	require.True(t, strings.Contains(w.Message, "3"), "message should carry the error count: %q", w.Message)
}
