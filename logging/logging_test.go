package logging

import (
	"bytes"
	"log"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWarnLogsInfoDoesNot(t *testing.T) {
	var buf bytes.Buffer
	prev := log.Writer()
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(prev) })

	Info("registered plugin: %s", "x")
	require.Empty(t, buf.String())

	Warn("unregister plugin: %s", "x")
	require.Contains(t, buf.String(), "WARN: unregister plugin: x")
}
