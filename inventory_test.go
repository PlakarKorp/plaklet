package plaklet

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	inventory "github.com/PlakarKorp/integration-sdk"
	"github.com/stretchr/testify/require"
)

type fakeInventory struct {
	entries []*inventory.InventoryEntry
	err     error
}

func (f *fakeInventory) List(_ context.Context, c chan<- *inventory.InventoryEntry) error {
	defer close(c)
	for _, e := range f.entries {
		c <- e
	}
	return f.err
}

func (f *fakeInventory) Close(context.Context) error { return nil }

func TestStreamEntries(t *testing.T) {
	bucket := &inventory.InventoryEntry{
		Class:    "object-storage",
		SubClass: "s3",
		Tags:     []string{"env=prod"},
		URN:      "urn:test:a",
		Name:     "a",
		Region:   "fr-par",
		Service:  "s3",
		Resource: "s3:bucket",
		Details:  []byte(`{"versioned":true}`),
		Endpoints: []inventory.HostEndpoint{{
			Type: inventory.EndpointHost, Endpoint: "a.example", Attributes: map[string]string{"port": "443"},
		}},
		Country: "FR",
	}
	boom := errors.New("boom")

	tests := []struct {
		name    string
		inv     *fakeInventory
		want    int
		wantErr error
	}{
		{"streams every entry", &fakeInventory{entries: []*inventory.InventoryEntry{bucket, bucket}}, 2, nil},
		{"a provider that does not sync fails the listing", &fakeInventory{err: inventory.ErrDisabledSync}, 0, inventory.ErrDisabledSync},
		{"a provider error fails the listing after its entries", &fakeInventory{entries: []*inventory.InventoryEntry{bucket}, err: boom}, 1, boom},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var replies []*ExecReply
			err := streamEntries(t.Context(), tt.inv, func(r *ExecReply) { replies = append(replies, r) })
			require.ErrorIs(t, err, tt.wantErr)
			require.Len(t, replies, tt.want)
			for _, r := range replies {
				require.Equal(t, ReplyInventory, r.Type)
				var got InventoryEntry
				require.NoError(t, json.Unmarshal(r.Report, &got))
				require.Equal(t, *wireEntry(bucket), got)
			}
		})
	}
}

// The wire keys are what plakman decodes; a rename here is a silent empty
// import there.
func TestInventoryEntryWireKeys(t *testing.T) {
	raw, err := json.Marshal(wireEntry(&inventory.InventoryEntry{
		Class: "compute", URN: "urn:x", Name: "x",
		Endpoints: []inventory.HostEndpoint{{Type: inventory.EndpointInet4, Endpoint: "10.0.0.1"}},
	}))
	require.NoError(t, err)
	var keys map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &keys))
	for _, k := range []string{"class", "urn", "name", "endpoints"} {
		require.Contains(t, keys, k)
	}
}

func TestInventoryNeedsAnInventorySource(t *testing.T) {
	err := listInventory(newTestContext(t), &ExecPayload{Op: "inventory"}, func(*ExecReply) {})
	require.Error(t, err)
	err = listInventory(newTestContext(t), &ExecPayload{Op: "inventory", Source: &Configuration{Type: "store"}}, func(*ExecReply) {})
	require.Error(t, err)
}
