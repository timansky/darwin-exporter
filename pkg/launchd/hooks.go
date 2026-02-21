//go:build darwin

package launchd

import "fmt"

// Hooks defines optional callbacks for user-visible messages.
// All callbacks are optional; nil callbacks are ignored.
type Hooks struct {
	Infof    func(format string, args ...any)
	Successf func(format string, args ...any)
	Warnf    func(format string, args ...any)

	StyleInfo    func(string) string
	StyleWarn    func(string) string
	StyleError   func(string) string
	StyleKey     func(string) string
	StyleSuccess func(string) string

	Printf  func(format string, args ...any)
	Println func(args ...any)
	Print   func(args ...any)
}

func (h Hooks) infof(format string, args ...any) {
	if h.Infof != nil {
		h.Infof(format, args...)
	}
}

func (h Hooks) successf(format string, args ...any) {
	if h.Successf != nil {
		h.Successf(format, args...)
	}
}

func (h Hooks) warnf(format string, args ...any) {
	if h.Warnf != nil {
		h.Warnf(format, args...)
	}
}

func (h Hooks) styleInfo(s string) string {
	if h.StyleInfo != nil {
		return h.StyleInfo(s)
	}
	return s
}

func (h Hooks) styleWarn(s string) string {
	if h.StyleWarn != nil {
		return h.StyleWarn(s)
	}
	return s
}

func (h Hooks) styleError(s string) string {
	if h.StyleError != nil {
		return h.StyleError(s)
	}
	return s
}

func (h Hooks) styleKey(s string) string {
	if h.StyleKey != nil {
		return h.StyleKey(s)
	}
	return s
}

func (h Hooks) styleSuccess(s string) string {
	if h.StyleSuccess != nil {
		return h.StyleSuccess(s)
	}
	return s
}

func (h Hooks) printf(format string, args ...any) {
	if h.Printf != nil {
		h.Printf(format, args...)
		return
	}
	fmt.Printf(format, args...)
}

func (h Hooks) println(args ...any) {
	if h.Println != nil {
		h.Println(args...)
		return
	}
	fmt.Println(args...)
}

func (h Hooks) print(args ...any) {
	if h.Print != nil {
		h.Print(args...)
		return
	}
	fmt.Print(args...)
}
