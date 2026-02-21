//go:build darwin

package cmd

import (
	"fmt"

	launchd "github.com/timansky/darwin-exporter/pkg/launchd"
)

func serviceHooks() launchd.Hooks {
	return launchd.Hooks{
		Infof:        printInfof,
		Successf:     printSuccessf,
		Warnf:        printWarnf,
		StyleInfo:    styleInfo,
		StyleWarn:    styleWarn,
		StyleError:   styleError,
		StyleKey:     styleKey,
		StyleSuccess: styleSuccess,
		Printf: func(format string, args ...any) {
			_, _ = fmt.Printf(format, args...)
		},
		Println: func(args ...any) {
			_, _ = fmt.Println(args...)
		},
		Print: func(args ...any) {
			_, _ = fmt.Print(args...)
		},
	}
}
