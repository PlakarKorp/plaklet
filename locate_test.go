package plaklet

import (
	"testing"
	"time"

	"github.com/PlakarKorp/kloset/locate"
	"github.com/stretchr/testify/require"
)

// The maps in these tests are the exact key shapes plakman's contract Flatten
// methods emit (flatten.PutStringList writes "key.0", "key.1", ...; PutBool
// writes "true"/"false"; PutTime writes RFC3339): what executor/forward.go
// hands the edge verbatim.

func TestLocateOptionsFilterNamespace(t *testing.T) {
	lo := locateOptions(map[string]string{
		"@version":                     "1",
		"locate.filter.before":         "2026-01-02T03:04:05Z",
		"locate.filter.since":          "2025-01-02T03:04:05Z",
		"locate.filter.name":           "nightly-name",
		"locate.filter.category":       "db",
		"locate.filter.environment":    "prod",
		"locate.filter.perimeter":      "eu",
		"locate.filter.job":            "job-1",
		"locate.filter.dataset":        "ds-1",
		"locate.filter.tags.0":         "nightly",
		"locate.filter.tags.1":         "weekly",
		"locate.filter.ignore_tags.0":  "scratch",
		"locate.filter.latest":         "true",
		"locate.filter.ids.0":          "abc123",
		"locate.filter.types.0":        "fs",
		"locate.filter.origins.0":      "host-a",
		"locate.filter.roots.0":        "/srv/data",
		"locate.filter.data_classes.0": "pii",
		"locate.group_by":              "name",
		"locate.period.day.keep":       "7",
		"locate.period.day.cap":        "2",
		"locate.period.week.keep":      "4",
		"locate.period.monday.keep":    "1",
	})

	require.Equal(t, time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC), lo.Filters.Before)
	require.Equal(t, time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC), lo.Filters.Since)
	require.Equal(t, "nightly-name", lo.Filters.Name)
	require.Equal(t, "db", lo.Filters.Category)
	require.Equal(t, "prod", lo.Filters.Environment)
	require.Equal(t, "eu", lo.Filters.Perimeter)
	require.Equal(t, "job-1", lo.Filters.Job)
	require.Equal(t, "ds-1", lo.Filters.Dataset)
	require.Equal(t, []string{"nightly", "weekly"}, lo.Filters.Tags)
	require.Equal(t, []string{"scratch"}, lo.Filters.IgnoreTags)
	require.True(t, lo.Filters.Latest)
	require.Equal(t, []string{"abc123"}, lo.Filters.IDs)
	require.Equal(t, []string{"fs"}, lo.Filters.Types)
	require.Equal(t, []string{"host-a"}, lo.Filters.Origins)
	require.Equal(t, []string{"/srv/data"}, lo.Filters.Roots)
	require.Equal(t, []string{"pii"}, lo.Filters.DataClasses)
	require.Equal(t, locate.GroupByKey("name"), lo.GroupBy)
	require.Equal(t, locate.LocatePeriod{Keep: 7, Cap: 2}, lo.Periods.Day)
	require.Equal(t, locate.LocatePeriod{Keep: 4}, lo.Periods.Week)
	require.Equal(t, locate.LocatePeriod{Keep: 1}, lo.Periods.Monday)
}

// TestLocateOptionsTopLevelFolds mirrors what plakman stores for a restore:
// the contract's Expand folds the task-level labels/latest/snapshot_id into
// the locate filters and the re-flattened map carries both forms, the
// task-level one winning. A config carrying only the task-level keys (a
// request that never named locate filters) must resolve the same way.
func TestLocateOptionsTopLevelFolds(t *testing.T) {
	// Both forms present (a stored, re-flattened restore config).
	lo := locateOptions(map[string]string{
		"labels.0":             "nightly",
		"locate.filter.tags.0": "nightly",
		"latest":               "true",
		"locate.filter.latest": "true",
		"snapshot_id":          "abc123",
		"locate.filter.ids.0":  "abc123",
	})
	require.Equal(t, []string{"nightly"}, lo.Filters.Tags)
	require.True(t, lo.Filters.Latest)
	require.Equal(t, []string{"abc123"}, lo.Filters.IDs)

	// Task-level keys alone.
	lo = locateOptions(map[string]string{
		"labels.0":    "nightly",
		"labels.1":    "weekly",
		"latest":      "true",
		"snapshot_id": "abc123",
	})
	require.Equal(t, []string{"nightly", "weekly"}, lo.Filters.Tags)
	require.True(t, lo.Filters.Latest)
	require.Equal(t, []string{"abc123"}, lo.Filters.IDs)

	// The task-level fold wins over the locate namespace when they disagree,
	// matching the contract Expand's overwrite.
	lo = locateOptions(map[string]string{
		"labels.0":             "new",
		"locate.filter.tags.0": "old",
		"latest":               "false",
		"locate.filter.latest": "true",
		"snapshot_id":          "new-id",
		"locate.filter.ids.0":  "old-id",
	})
	require.Equal(t, []string{"new"}, lo.Filters.Tags)
	require.False(t, lo.Filters.Latest)
	require.Equal(t, []string{"new-id"}, lo.Filters.IDs)
}

// TestLocateOptionsRelativeWindows: "before"/"after" are windows in seconds
// relative to now, applied on top of any absolute locate.filter times, the way
// plakman's taskConfigToLocateOptions anchors them at dispatch.
func TestLocateOptionsRelativeWindows(t *testing.T) {
	start := time.Now()
	lo := locateOptions(map[string]string{
		"locate.filter.before": "2020-01-02T03:04:05Z",
		"before":               "3600",
		"after":                "86400",
	})
	end := time.Now()

	require.WithinRange(t, lo.Filters.Before,
		start.Add(-3600*time.Second), end.Add(-3600*time.Second))
	require.WithinRange(t, lo.Filters.Since,
		start.Add(-86400*time.Second), end.Add(-86400*time.Second))
}

func TestLocateOptionsEmpty(t *testing.T) {
	lo := locateOptions(map[string]string{})
	require.Empty(t, lo.Filters.IDs)
	require.False(t, lo.Filters.Latest)
	require.Empty(t, lo.Filters.Tags)
	require.True(t, lo.Filters.Before.IsZero())
	require.True(t, lo.Filters.Since.IsZero())
	require.False(t, lo.HasPeriods())
}
