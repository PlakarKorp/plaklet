package plaklet

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	sdk "github.com/PlakarKorp/integration-sdk"
	"github.com/PlakarKorp/kloset/kcontext"
	"github.com/PlakarKorp/pkg"
)

func mkinventory(ctx *kcontext.KContext, conf *Configuration) (sdk.Inventory, error) {
	p, err := pluginFor(conf, pkg.ConnectorTypeInventory)
	if err != nil {
		return nil, err
	}
	return p.NewInventory(ctx, protocolOf(conf), conf.params())
}

// listInventory runs the inventory plugin the source names and streams every
// entry it lists as a ReplyInventory. It produces no report: the entries are
// the result, and the driver imports them as they arrive.
func listInventory(ctx *kcontext.KContext, input *ExecPayload, send func(*ExecReply)) error {
	if input.Source == nil {
		return errors.New("source must be set for inventory")
	}
	if input.Source.Type != "inventory" {
		return fmt.Errorf("inventory: %s: invalid source type", input.Source.Type)
	}

	inv, err := mkinventory(ctx, input.Source)
	if err != nil {
		return fmt.Errorf("start inventory plugin %s: %w", input.Source.Integration.Name, err)
	}
	defer ensureClose(ctx, inv)

	return streamEntries(ctx.Context, inv, send)
}

// streamEntries lists inv and sends each entry as it comes. Any error the
// provider ends on fails the listing, a provider that does not sync included:
// the driver imports what it received as the inventory's new state, and an
// empty success would empty the inventory.
func streamEntries(ctx context.Context, inv sdk.Inventory, send func(*ExecReply)) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	entries := make(chan *sdk.InventoryEntry)
	done := make(chan error, 1)
	go func() {
		defer close(done)
		for e := range entries {
			raw, err := json.Marshal(wireEntry(e))
			if err != nil {
				done <- fmt.Errorf("encode inventory entry %s: %w", e.URN, err)
				// Stop the provider, and keep draining so it never blocks on
				// a channel nobody reads while it winds down.
				cancel()
				for range entries {
				}
				return
			}
			send(&ExecReply{Type: ReplyInventory, Report: raw})
		}
	}()

	err := inv.List(ctx, entries)
	if sendErr := <-done; sendErr != nil {
		return sendErr
	}
	return err
}

func wireEntry(e *sdk.InventoryEntry) *InventoryEntry {
	entry := &InventoryEntry{
		Class:    string(e.Class),
		SubClass: string(e.SubClass),
		Tags:     e.Tags,
		URN:      e.URN,
		Name:     e.Name,
		Region:   e.Region,
		Service:  e.Service,
		Resource: e.Resource,
		Details:  e.Details,
		Country:  string(e.Country),
	}
	for _, ep := range e.Endpoints {
		entry.Endpoints = append(entry.Endpoints, InventoryEndpoint{
			Type:       string(ep.Type),
			Endpoint:   ep.Endpoint,
			Attributes: ep.Attributes,
		})
	}
	return entry
}
