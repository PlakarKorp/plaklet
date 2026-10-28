package plaklet

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// runMain feeds payload to Main on stdin and returns the replies it streamed
// on stdout. Main owns the process stdio, so tests using it can't be parallel.
func runMain(t *testing.T, payload *ExecPayload) []ExecReply {
	t.Helper()

	stdinR, stdinW, err := os.Pipe()
	require.NoError(t, err)
	stdoutR, stdoutW, err := os.Pipe()
	require.NoError(t, err)

	origStdin, origStdout := os.Stdin, os.Stdout
	os.Stdin, os.Stdout = stdinR, stdoutW
	t.Cleanup(func() { os.Stdin, os.Stdout = origStdin, origStdout })

	require.NoError(t, json.NewEncoder(stdinW).Encode(payload))
	require.NoError(t, stdinW.Close())

	var replies []ExecReply
	read := make(chan error, 1)
	go func() {
		scanner := bufio.NewScanner(stdoutR)
		scanner.Buffer(nil, 1<<20)
		for scanner.Scan() {
			var r ExecReply
			if err := json.Unmarshal(scanner.Bytes(), &r); err != nil {
				read <- err
				return
			}
			replies = append(replies, r)
		}
		read <- scanner.Err()
	}()

	// -cpu 0 keeps Main from changing GOMAXPROCS for the whole test binary.
	code := Main([]string{"-cache", t.TempDir(), "-cpu", "0", "-quiet"})
	require.NoError(t, stdoutW.Close())
	require.NoError(t, <-read)
	require.NoError(t, stdinR.Close())
	require.NoError(t, stdoutR.Close())
	require.Equal(t, 0, code)

	return replies
}

// A failed operation's last state says why it failed, so a consumer that only
// keeps the state does not see a clean run next to a failed job.
func TestMainFailureFinalStateCarriesError(t *testing.T) {
	repoDir := filepath.Join(t.TempDir(), "repo")
	createFSRepo(t, newTestContext(t), repoDir)

	replies := runMain(t, &ExecPayload{
		Op:     "restore",
		Source: fsConf(uuid.New(), "storage", repoDir),
		Target: fsConf(uuid.New(), "exporter", t.TempDir()),
	})

	require.NotEmpty(t, replies)
	last := replies[len(replies)-1]
	require.Equal(t, ReplyFailure, last.Type)
	require.Contains(t, last.Message, "no snapshots found")

	var final *State
	for _, r := range replies {
		if r.Type != ReplyState {
			continue
		}
		var st State
		require.NoError(t, json.Unmarshal(r.State, &st))
		final = &st
	}
	require.NotNil(t, final, "no state streamed")
	require.Equal(t, "no snapshots found", final.Error)
}
