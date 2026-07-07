// Command plaklet is the standalone executable wrapper around the plaklet
// package. All logic lives in the importable package so drivers such as
// plakar-edge can embed it; this wrapper simply runs it as its own binary.
package main

import (
	"os"

	"github.com/PlakarKorp/plaklet"
)

func main() {
	os.Exit(plaklet.Main(os.Args[1:]))
}
