package main

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestConfigParamsFlattensFields(t *testing.T) {
	c := &Configuration{
		Fields: []ConfigurationField{
			{Key: "location", Val: "fs:///data"},
			{Key: "passphrase", Val: "secret"},
		},
	}
	require.Equal(t, map[string]string{
		"location":   "fs:///data",
		"passphrase": "secret",
	}, c.params())
}

func TestConfigParamsEmpty(t *testing.T) {
	require.Empty(t, (&Configuration{}).params())
}

// The control plane sends connector fields as {key,val}; the Configuration type
// must decode that and re-encode into what plaklet feeds plaklet (provider
// present, defaulting to nil). This asserts the round-trip stays literal.
func TestConfigurationDecodesControlPlaneShape(t *testing.T) {
	// {key,val} only — no "provider" key, as the API sends.
	in := `{"integration":{"name":"fs","version":"1.1.2"},
	        "fields":[{"key":"location","val":"fs:///x"}]}`
	var c Configuration
	require.NoError(t, json.Unmarshal([]byte(in), &c))
	require.Equal(t, "fs", c.Integration.Name)
	require.Len(t, c.Fields, 1)
	require.Nil(t, c.Fields[0].Provider)
	require.Equal(t, "fs:///x", c.Fields[0].Val)
	require.Equal(t, map[string]string{"location": "fs:///x"}, c.params())
}

func TestExecReplyOmitsEmptyRawJSON(t *testing.T) {
	// A bare success reply should not carry null report/state fields.
	b, err := json.Marshal(&ExecReply{Type: ReplySuccess})
	require.NoError(t, err)
	require.JSONEq(t, `{"type":"success"}`, string(b))
}

func TestExecReplyCarriesReportRaw(t *testing.T) {
	b, err := json.Marshal(&ExecReply{Type: ReplyReport, Report: json.RawMessage(`{"type":"backup"}`)})
	require.NoError(t, err)
	require.JSONEq(t, `{"type":"report","report":{"type":"backup"}}`, string(b))
}
