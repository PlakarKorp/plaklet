package plaklet

import (
	"errors"

	"github.com/PlakarKorp/kloset/kcontext"
)

func maintenance(kcontext *kcontext.KContext, input *ExecPayload) (*Report, error) {
	if input.Source == nil {
		return nil, errors.New("target need to be set for maintenance")
	}

	return nil, errors.ErrUnsupported
}
