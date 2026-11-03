// Command plaklet is the standalone executable wrapper around the plaklet
// package. All logic lives in the importable package so drivers such as
// plakar-edge can embed it; this wrapper simply runs it as its own binary.
package main

import (
	"os"

	"github.com/PlakarKorp/plaklet"
)

// exit is os.Exit, replaced in tests to observe the exit code.
var exit = os.Exit

func main() {
	exit(plaklet.Main(os.Args[1:]))
}
