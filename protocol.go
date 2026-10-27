package plaklet

import (
	"encoding/json"

	"github.com/google/uuid"
)

// This file defines the stdin/stdout wire protocol plaklet speaks with whatever
// drives it (the plakman executor, or plakar-edge). The shapes are duplicated
// from plakman's executor.ExecPayload / executor.ExecReply and
// executor/contract.Configuration so plaklet stays free of any plakman
// dependency. Keep the JSON tags in lockstep with those types.

// ExecPayload is the single JSON object plaklet reads from stdin.
type ExecPayload struct {
	Op         string            `json:"op"`
	TaskConfig map[string]string `json:"task_config"`
	Source     *Configuration    `json:"source"`
	Target     *Configuration    `json:"target"`
}

// ReplyType enumerates the messages plaklet streams to stdout.
type ReplyType string

const (
	ReplyInfo    ReplyType = "info"
	ReplyWarning ReplyType = "warning"
	ReplyError   ReplyType = "error"
	ReplyReport  ReplyType = "report"
	ReplyState   ReplyType = "state"
	ReplyFailure ReplyType = "failure"
	ReplySuccess ReplyType = "success"
)

// ExecReply is one message in plaklet's stdout stream. Report is emitted as raw
// JSON so the shape is owned here and consumers forward it verbatim.
type ExecReply struct {
	Type    ReplyType       `json:"type"`
	Message string          `json:"message,omitempty"`
	Report  json.RawMessage `json:"report,omitempty"`
	State   json.RawMessage `json:"state,omitempty"`
}

// Configuration is a resolved connector configuration. Fields carry literal
// values (secrets are resolved by the caller before the payload reaches
// plaklet), so Provider is expected to be nil.
type Configuration struct {
	Id          uuid.UUID            `json:"id"`
	Revision    int                  `json:"revision"`
	Type        string               `json:"type"`
	Integration Integration          `json:"integration"`
	Name        string               `json:"name"`
	Fields      []ConfigurationField `json:"fields"`
	Environment string               `json:"environment,omitempty"`
	DataClasses []string             `json:"data_classes,omitempty"`
	URN         string               `json:"urn"`
	URNID       uuid.UUID            `json:"urnid"`
}

type Integration struct {
	Id      uuid.UUID `json:"id"`
	Name    string    `json:"name"`
	Version string    `json:"version"`
}

type ConfigurationField struct {
	Key      string         `json:"key"`
	Provider *Configuration `json:"provider"`
	Val      string         `json:"val"`
}

// params flattens a resolved configuration's fields into the key/value map the
// kloset connector registries expect. Provider references are ignored: by
// contract they are already resolved into literal Val values upstream.
func (c *Configuration) params() map[string]string {
	m := make(map[string]string, len(c.Fields))
	for _, f := range c.Fields {
		m[f.Key] = f.Val
	}
	return m
}

// ReplyInventory carries one inventory entry in Report while the "inventory"
// operation lists a provider. Additive: a driver that predates it drops the
// unknown type.
const ReplyInventory ReplyType = "inventory"

// InventoryEntry is the wire form of go-inventory-sdk's InventoryEntry, as
// the "inventory" operation emits it. SOURCE OF TRUTH -- plakman decodes it
// (github.com/PlakarKorp/plakman inventory/edge.go); keep the two in sync.
type InventoryEntry struct {
	Class     string              `json:"class"`
	SubClass  string              `json:"subclass,omitempty"`
	Tags      []string            `json:"tags,omitempty"`
	URN       string              `json:"urn"`
	Name      string              `json:"name"`
	Region    string              `json:"region,omitempty"`
	Service   string              `json:"service,omitempty"`
	Resource  string              `json:"resource,omitempty"`
	Details   []byte              `json:"details,omitempty"`
	Endpoints []InventoryEndpoint `json:"endpoints,omitempty"`
	Country   string              `json:"country,omitempty"`
}

type InventoryEndpoint struct {
	Type       string            `json:"type"`
	Endpoint   string            `json:"endpoint"`
	Attributes map[string]string `json:"attributes,omitempty"`
}
