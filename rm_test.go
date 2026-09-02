package plaklet

import (
	"encoding/hex"
	"strings"
	"testing"

	"github.com/PlakarKorp/kloset/objects"
	"github.com/stretchr/testify/require"
)

func TestParseSnapshotIDs(t *testing.T) {
	full := strings.Repeat("ab", 32)
	other := strings.Repeat("cd", 32)

	tests := []struct {
		name    string
		ids     []string
		wantLen int
		wantErr bool
	}{
		{name: "empty list", ids: nil, wantLen: 0},
		{name: "one identifier", ids: []string{full}, wantLen: 1},
		{name: "two identifiers", ids: []string{full, other}, wantLen: 2},
		{name: "not hexadecimal", ids: []string{strings.Repeat("zz", 32)}, wantErr: true},
		{name: "too short", ids: []string{strings.Repeat("ab", 16)}, wantErr: true},
		{name: "too long", ids: []string{strings.Repeat("ab", 33)}, wantErr: true},
		{name: "one bad identifier fails the list", ids: []string{full, "beef"}, wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			macs, err := parseSnapshotIDs(tc.ids)
			if tc.wantErr {
				require.Error(t, err)
				require.Nil(t, macs)
				return
			}
			require.NoError(t, err)
			require.Len(t, macs, tc.wantLen)
			for i, id := range tc.ids {
				raw, err := hex.DecodeString(id)
				require.NoError(t, err)
				var want objects.MAC
				copy(want[:], raw)
				require.Equal(t, want, macs[i])
			}
		})
	}
}

func TestIndexedList(t *testing.T) {
	require.Nil(t, indexedList(nil, "snapshot_ids"))
	require.Nil(t, indexedList(map[string]string{"other": "x"}, "snapshot_ids"))
	require.Equal(t, []string{"a"}, indexedList(map[string]string{"snapshot_ids.0": "a"}, "snapshot_ids"))
	require.Equal(t, []string{"a", "b"},
		indexedList(map[string]string{"snapshot_ids.0": "a", "snapshot_ids.1": "b"}, "snapshot_ids"))
	// A hole ends the list: flatten writes contiguous indices.
	require.Equal(t, []string{"a"},
		indexedList(map[string]string{"snapshot_ids.0": "a", "snapshot_ids.2": "c"}, "snapshot_ids"))
}
