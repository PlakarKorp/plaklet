package plaklet

import (
	"bytes"
	"fmt"
	"hash"
	"io"
	"strings"

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
// / "no_compression" ("true") turn those off, and "compression" names the
// algorithm to use instead of kloset's default. This mirrors the plakman
// plaklet's create op so an edge can initialize the store it will back up into.
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
	switch {
	case input.TaskConfig["no_compression"] == "true":
		cfg.Compression = nil
	case input.TaskConfig["compression"] != "":
		compressionConfiguration, err := compression.LookupDefaultConfiguration(strings.ToUpper(input.TaskConfig["compression"]))
		if err != nil {
			return nil, fmt.Errorf("compression algorithm %q: %w", input.TaskConfig["compression"], err)
		}
		cfg.Compression = compressionConfiguration
	default:
		// No preference: kloset keeps the say on its own default.
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
