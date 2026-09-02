package plaklet

import (
	"strconv"
	"time"

	"github.com/PlakarKorp/kloset/locate"
)

// locateOptions builds kloset locate options from the flat task config map.
//
// The control plane flattens its typed task configs with plakman's
// flatten.Flattener before dispatch: []string values arrive as "key.0",
// "key.1", ..., booleans as "true"/"false", times as RFC3339, and every
// locate filter lives under the "locate.filter." prefix (see
// contract.LocateConfig.Flatten in plakman). On top of that namespace the
// task types keep a few top-level convenience keys that plakman's Expand
// methods fold into the locate filters; the same folds are applied here, in
// the same order, so a config naming both forms resolves the same way:
//
//	labels.N     replaces the tag filter (restore/check/sync)
//	latest       overrides locate.filter.latest (restore)
//	snapshot_id  replaces the id filter (restore/check)
//	before/after windows in seconds relative to now (restore/check/sync)
//
// This is the standalone equivalent of plakman's taskConfigToLocateOptions.
func locateOptions(cfg map[string]string) *locate.LocateOptions {
	lo := locate.NewDefaultLocateOptions()

	str := func(key string, dst *string) {
		if v, ok := cfg[key]; ok {
			*dst = v
		}
	}
	boolean := func(key string, dst *bool) {
		if v, ok := cfg[key]; ok {
			if b, err := strconv.ParseBool(v); err == nil {
				*dst = b
			}
		}
	}
	when := func(key string, dst *time.Time) {
		if v, ok := cfg[key]; ok {
			if ts, err := time.Parse(time.RFC3339, v); err == nil {
				*dst = ts
			}
		}
	}

	when("locate.filter.before", &lo.Filters.Before)
	when("locate.filter.since", &lo.Filters.Since)
	str("locate.filter.name", &lo.Filters.Name)
	str("locate.filter.category", &lo.Filters.Category)
	str("locate.filter.environment", &lo.Filters.Environment)
	str("locate.filter.perimeter", &lo.Filters.Perimeter)
	str("locate.filter.job", &lo.Filters.Job)
	str("locate.filter.dataset", &lo.Filters.Dataset)
	lo.Filters.Tags = indexedList(cfg, "locate.filter.tags")
	lo.Filters.IgnoreTags = indexedList(cfg, "locate.filter.ignore_tags")
	boolean("locate.filter.latest", &lo.Filters.Latest)
	lo.Filters.IDs = indexedList(cfg, "locate.filter.ids")
	lo.Filters.Types = indexedList(cfg, "locate.filter.types")
	lo.Filters.Origins = indexedList(cfg, "locate.filter.origins")
	lo.Filters.Roots = indexedList(cfg, "locate.filter.roots")
	lo.Filters.DataClasses = indexedList(cfg, "locate.filter.data_classes")
	if v, ok := cfg["locate.group_by"]; ok {
		lo.GroupBy = locate.GroupByKey(v)
	}

	period := func(name string, dst *locate.LocatePeriod) {
		if v, ok := cfg["locate.period."+name+".keep"]; ok {
			if n, err := strconv.Atoi(v); err == nil {
				dst.Keep = n
			}
		}
		if v, ok := cfg["locate.period."+name+".cap"]; ok {
			if n, err := strconv.Atoi(v); err == nil {
				dst.Cap = n
			}
		}
	}
	period("minute", &lo.Periods.Minute)
	period("hour", &lo.Periods.Hour)
	period("day", &lo.Periods.Day)
	period("week", &lo.Periods.Week)
	period("month", &lo.Periods.Month)
	period("year", &lo.Periods.Year)
	period("monday", &lo.Periods.Monday)
	period("tuesday", &lo.Periods.Tuesday)
	period("wednesday", &lo.Periods.Wednesday)
	period("thursday", &lo.Periods.Thursday)
	period("friday", &lo.Periods.Friday)
	period("saturday", &lo.Periods.Saturday)
	period("sunday", &lo.Periods.Sunday)

	// Top-level folds, after the locate.filter namespace so they win, the way
	// the contract's Expand methods overwrite the expanded locate config.
	if labels := indexedList(cfg, "labels"); len(labels) != 0 {
		lo.Filters.Tags = labels
	}
	boolean("latest", &lo.Filters.Latest)
	if id := cfg["snapshot_id"]; id != "" {
		lo.Filters.IDs = []string{id}
	}

	now := time.Now()
	if v, ok := cfg["before"]; ok {
		if secs, err := strconv.ParseUint(v, 10, 64); err == nil {
			lo.Filters.Before = now.Add(-time.Duration(secs) * time.Second)
		}
	}
	if v, ok := cfg["after"]; ok {
		if secs, err := strconv.ParseUint(v, 10, 64); err == nil {
			lo.Filters.Since = now.Add(-time.Duration(secs) * time.Second)
		}
	}

	return lo
}
