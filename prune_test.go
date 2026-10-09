package plaklet

import (
	"testing"

	"github.com/PlakarKorp/kloset/locate"
	"github.com/stretchr/testify/require"
)

// The UI starts prunes with no group_by; retention must still be partitioned
// per source so a store fed by several datasets does not have one source's
// snapshots evicted by another's. defaultPruneGroupByDataset supplies that
// default, but only where group_by is meaningful (a period is set) and never
// over an explicit choice.
func TestDefaultPruneGroupByDataset(t *testing.T) {
	withWeek := func() locate.LocatePeriods {
		var p locate.LocatePeriods
		p.Week.Keep = 4
		p.Week.Cap = 1
		return p
	}

	tests := []struct {
		name string
		lo   locate.LocateOptions
		want locate.GroupByKey
	}{
		{
			name: "period set, no group_by -> dataset",
			lo:   locate.LocateOptions{Periods: withWeek()},
			want: locate.GroupByDataset,
		},
		{
			name: "no period -> left as none",
			lo:   locate.LocateOptions{},
			want: locate.GroupByNone,
		},
		{
			name: "explicit group_by is preserved",
			lo:   locate.LocateOptions{Periods: withWeek(), GroupBy: locate.GroupByName},
			want: locate.GroupByName,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			lo := tc.lo
			defaultPruneGroupByDataset(&lo)
			require.Equal(t, tc.want, lo.GroupBy)
		})
	}
}
