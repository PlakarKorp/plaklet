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
	Id          string               `json:"id"`
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
	Id      string `json:"id"`
	Name    string `json:"name"`
	Version string `json:"version"`
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
