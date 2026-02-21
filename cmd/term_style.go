//go:build darwin

package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/muesli/termenv"
)

const (
	ansiReset  = "\x1b[0m"
	ansiRed    = "\x1b[31m"
	ansiGreen  = "\x1b[32m"
	ansiYellow = "\x1b[33m"
	ansiBlue   = "\x1b[34m"
	ansiCyan   = "\x1b[36m"
)

var colorEnabled bool
var termProfile = termenv.EnvColorProfile()

func init() {
	SetColorMode("auto")
}

// SetColorMode sets output coloring mode: auto|always|never.
func SetColorMode(mode string) {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "always":
		colorEnabled = true
	case "never":
		colorEnabled = false
	default:
		colorEnabled = detectAutoColor()
	}
}

func detectAutoColor() bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	term := strings.TrimSpace(strings.ToLower(os.Getenv("TERM")))
	if term == "" || term == "dumb" {
		return false
	}
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

func colorize(text, ansi string) string {
	if !colorEnabled {
		return text
	}
	switch ansi {
	case ansiRed:
		return termenv.String(text).Foreground(termenv.ANSIRed).String()
	case ansiGreen:
		return termenv.String(text).Foreground(termenv.ANSIGreen).String()
	case ansiYellow:
		return termenv.String(text).Foreground(termenv.ANSIYellow).String()
	case ansiBlue:
		return termenv.String(text).Foreground(termenv.ANSIBlue).String()
	case ansiCyan:
		return termenv.String(text).Foreground(termenv.ANSICyan).String()
	default:
		return termProfile.String(text).String()
	}
}

func styleSuccess(text string) string { return colorize(text, ansiGreen) }
func styleInfo(text string) string    { return colorize(text, ansiCyan) }
func styleWarn(text string) string    { return colorize(text, ansiYellow) }
func styleError(text string) string   { return colorize(text, ansiRed) }
func styleKey(text string) string     { return colorize(text, ansiBlue) }

func printSuccessf(format string, args ...any) {
	fmt.Println(styleSuccess(fmt.Sprintf(format, args...)))
}

func printInfof(format string, args ...any) {
	fmt.Println(styleInfo(fmt.Sprintf(format, args...)))
}

func printWarnf(format string, args ...any) {
	fmt.Println(styleWarn(fmt.Sprintf(format, args...)))
}
