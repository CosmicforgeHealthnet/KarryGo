package logging

import (
	"fmt"
	"log"
)

const (
	reset  = "\033[0m"
	red    = "\033[31m"
	green  = "\033[32m"
	yellow = "\033[33m"
	bold   = "\033[1m"
)

// Success logs a green ✅ line. Use for healthy connectivity and successful operations.
func Success(scope, format string, args ...any) {
	log.Printf("%s✅ %s%s%s %s", green, bold, scope, reset, fmt.Sprintf(format, args...))
}

// Error logs a red ❌ line. Use for non-fatal errors.
func Error(scope, format string, args ...any) {
	log.Printf("%s❌ %s%s%s %s", red, bold, scope, reset, fmt.Sprintf(format, args...))
}

// Notice logs a yellow ⚠️  line. Use for degraded/optional config and informational startup state.
func Notice(scope, format string, args ...any) {
	log.Printf("%s⚠️  %s%s%s %s", yellow, bold, scope, reset, fmt.Sprintf(format, args...))
}

// Fatal logs a red 💥 line then calls log.Fatalf.
func Fatal(scope, format string, args ...any) {
	log.Fatalf("%s💥 %s%s%s %s", red, bold, scope, reset, fmt.Sprintf(format, args...))
}
