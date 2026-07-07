package plaklet

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLocateOptionsFromConfig(t *testing.T) {
	lo := locateOptions(map[string]string{
		"snapshot": "abc123",
		"latest":   "true",
		"tags":     "nightly, weekly ,",
	})
	require.Equal(t, []string{"abc123"}, lo.Filters.IDs)
	require.True(t, lo.Filters.Latest)
	require.Equal(t, []string{"nightly", "weekly"}, lo.Filters.Tags)
}

func TestLocateOptionsEmpty(t *testing.T) {
	lo := locateOptions(map[string]string{})
	require.Empty(t, lo.Filters.IDs)
	require.False(t, lo.Filters.Latest)
	require.Empty(t, lo.Filters.Tags)
}

func TestSplitList(t *testing.T) {
	require.Nil(t, splitList(""))
	require.Equal(t, []string{"a"}, splitList("a"))
	require.Equal(t, []string{"a", "b"}, splitList(" a , b "))
	require.Nil(t, splitList(" , , "))
}
