package plaklet

import (
	"errors"

	"github.com/PlakarKorp/kloset/kcontext"
	"github.com/PlakarKorp/kloset/locate"
)

func prune(kcontext *kcontext.KContext, input *ExecPayload) (*Report, error) {
	if input.Source == nil {
		return nil, errors.New("target need to be set for prune")
	}

	return nil, errors.ErrUnsupported
}

// defaultPruneGroupByDataset makes prune retention per-dataset by default: a
// store fed by several sources should honor its periods per source, not across
// all sources mixed. group_by only affects period retention (kloset treats it
// as a no-op without periods), so it is applied only when a period is set, and
// never overrides a group_by chosen explicitly upstream.
func defaultPruneGroupByDataset(lo *locate.LocateOptions) {
	if lo.GroupBy == locate.GroupByNone && lo.HasPeriods() {
		lo.GroupBy = locate.GroupByDataset
	}
}
