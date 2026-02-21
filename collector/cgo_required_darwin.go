//go:build darwin && !cgo

package collector

// This symbol intentionally does not exist to produce a clear compile-time
// error when CGO is disabled on macOS.
var _ = darwinExporterRequiresCGOEnabled1AndBuildTagCgo

// Keep !cgo builds from cascading into unrelated "undefined" errors in thermal
// collector code; the guard above is the intended actionable failure.
func discoverSMCKeySets() ([]string, []string, []string, error) {
	return nil, nil, nil, nil
}

func readSMCKeyValues(keys []string) (map[string]float64, error) {
	return nil, nil
}
