package plaklet

import (
	"bytes"
	"fmt"
	"hash"
	"io"

	"github.com/PlakarKorp/kloset/compression"
	"github.com/PlakarKorp/kloset/connectors/storage"
	"github.com/PlakarKorp/kloset/encryption"
	"github.com/PlakarKorp/kloset/hashing"
	"github.com/PlakarKorp/kloset/kcontext"
	"github.com/PlakarKorp/kloset/resources"
	"github.com/PlakarKorp/kloset/versioning"
)

// create initializes a new kloset store at the source location. Encryption is on
// by default when the store's passphrase field is set; task_config "no_encryption"
// / "no_compression" ("true") turn those off. This mirrors the plakman plaklet's
// create op so an edge can initialize the store it will back up into.
func create(ctx *kcontext.KContext, input *ExecPayload) (*Report, error) {
	if input.Source == nil {
		return nil, fmt.Errorf("source must be set for create")
	}

	store, passphrase, _, err := mkstorage(ctx, input.Source)
	if err != nil {
		return nil, err
	}
	defer store.Close(ctx)

	cfg := storage.NewConfiguration()
	if input.TaskConfig["no_compression"] == "true" {
		cfg.Compression = nil
	} else {
		cfg.Compression = compression.NewDefaultConfiguration()
	}

	var hasher hash.Hash
	if input.TaskConfig["no_encryption"] == "true" || passphrase == "" {
		cfg.Encryption = nil
		hasher = hashing.GetHasher(storage.DEFAULT_HASHING_ALGORITHM)
	} else {
		key, err := encryption.DeriveKey(cfg.Encryption.KDFParams, []byte(passphrase))
		if err != nil {
			return nil, err
		}
		canary, err := encryption.DeriveCanary(cfg.Encryption, key)
		if err != nil {
			return nil, err
		}
		cfg.Encryption.Canary = canary
		hasher = hashing.GetMACHasher(storage.DEFAULT_HASHING_ALGORITHM, key)
	}

	serialized, err := cfg.ToBytes()
	if err != nil {
		return nil, err
	}
	rd, err := storage.Serialize(hasher, resources.RT_CONFIG,
		versioning.GetCurrentVersion(resources.RT_CONFIG), bytes.NewReader(serialized))
	if err != nil {
		return nil, err
	}
	wrapped, err := io.ReadAll(rd)
	if err != nil {
		return nil, err
	}

	if err := store.Create(ctx, wrapped); err != nil {
		return nil, err
	}

	// create emits no report; a success reply is enough.
	return nil, nil
}
