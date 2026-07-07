// Package logging is a minimal stand-in for plakman's daemonize/logging, so the
// copied plugin package stays dependency-free. plaklet logs to stderr; these go
// there too, prefixed, without pulling in a logging framework.
package logging

import "log"

func Info(format string, args ...any) { log.Printf("plugin: "+format, args...) }
func Warn(format string, args ...any) { log.Printf("plugin: WARN "+format, args...) }
