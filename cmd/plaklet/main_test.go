package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// Without -cache the executor refuses to run; main must hand that exit code
// to the process.
func TestMainExitsWithTheExecutorCode(t *testing.T) {
	prevArgs, prevExit, prevErr := os.Args, exit, os.Stderr
	t.Cleanup(func() { os.Args, exit, os.Stderr = prevArgs, prevExit, prevErr })

	stderr, err := os.Create(filepath.Join(t.TempDir(), "stderr"))
	require.NoError(t, err)
	defer stderr.Close()
	os.Stderr = stderr

	var code int
	exit = func(c int) { code = c }
	os.Args = []string{"plaklet"}
	main()
	require.Equal(t, 2, code)
}
