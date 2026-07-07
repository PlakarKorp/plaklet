// Package logging is a minimal stand-in for plakman's daemonize/logging, so the
// copied plugin package stays dependency-free.
//
// Info is a no-op: plugin register/unregister is routine bookkeeping that only
// adds noise to plaklet's stdout/stderr (plaklet is spawned per task). Warn
// still surfaces genuine problems to stderr.
package logging

import "log"

func Info(string, ...any) {}

func Warn(format string, args ...any) { log.Printf("plugin: WARN "+format, args...) }
