package main

import "github.com/PlakarKorp/kloset/locate"

// locateOptions builds kloset locate options from the flat task config map. It
// supports the common selectors an operation needs:
//
//	snapshot   a specific snapshot ID (hex prefix)
//	latest     "true" to select only the most recent match
//	tags       comma-separated tag filter
//
// This is the standalone equivalent of plakman's taskConfigToLocateOptions; the
// full retention/group-by machinery is out of scope here.
func locateOptions(cfg map[string]string) *locate.LocateOptions {
	lo := locate.NewDefaultLocateOptions()

	if id := cfg["snapshot"]; id != "" {
		lo.Filters.IDs = []string{id}
	}
	if cfg["latest"] == "true" {
		lo.Filters.Latest = true
	}
	if tags := splitList(cfg["tags"]); len(tags) != 0 {
		lo.Filters.Tags = tags
	}

	return lo
}
