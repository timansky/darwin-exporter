//go:build darwin

package launchd

import (
	"bytes"
	"fmt"
	"path/filepath"
	"text/template"
)

// PlistLabel is the launchd service label used in both LaunchAgent and LaunchDaemon plists.
const PlistLabel = "kz.neko.darwin-exporter"

// plistTmpl is the embedded plist template.
// It generates an Apple property list for launchd.
const plistTmpl = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>{{.Label}}</string>
	<key>ProgramArguments</key>
	<array>
		<string>{{.BinaryPath}}</string>
		{{- if .Config}}
		<string>--config={{.Config}}</string>
		{{- end}}
	</array>
	<key>KeepAlive</key>
	<dict>
		<key>SuccessfulExit</key>
		<false/>
	</dict>
	<key>RunAtLoad</key>
	<true/>
	{{- if .LogDir}}
	<key>StandardOutPath</key>
	<string>{{.LogDir}}/darwin-exporter.log</string>
	<key>StandardErrorPath</key>
	<string>{{.LogDir}}/darwin-exporter.err</string>
	{{- end}}
	{{- if .RunAsRoot}}
	<key>UserName</key>
	<string>root</string>
	{{- end}}
</dict>
</plist>
`

// PlistParams holds the template variables for plist generation.
type PlistParams struct {
	Label      string // launchd label (e.g. "kz.neko.darwin-exporter")
	BinaryPath string // absolute path to the darwin-exporter binary
	Config     string // optional: path to config YAML
	LogDir     string // optional: directory for stdout/stderr logs
	RunAsRoot  bool   // true for LaunchDaemon (adds UserName=root key)
}

// ValidatePath checks that p is an absolute filesystem path containing only
// characters that are valid in macOS paths and safe for embedding in XML.
// Characters <, >, &, ", ' are forbidden because they have special meaning
// in XML/plist and are never required in filesystem paths.
func ValidatePath(p string) error {
	if !filepath.IsAbs(p) {
		return fmt.Errorf("path must be absolute: %q", p)
	}
	for _, ch := range p {
		if ch == '<' || ch == '>' || ch == '&' || ch == '"' || ch == '\'' {
			return fmt.Errorf("invalid character %q in path %q", ch, p)
		}
	}
	return nil
}

// GeneratePlist renders the plist template with the given parameters.
func GeneratePlist(p PlistParams) ([]byte, error) {
	if p.Label == "" {
		p.Label = PlistLabel
	}
	if p.BinaryPath == "" {
		return nil, fmt.Errorf("BinaryPath must not be empty")
	}

	// Validate paths before embedding them in XML to prevent XML injection.
	if err := ValidatePath(p.BinaryPath); err != nil {
		return nil, fmt.Errorf("invalid BinaryPath: %w", err)
	}
	if p.Config != "" {
		if err := ValidatePath(p.Config); err != nil {
			return nil, fmt.Errorf("invalid Config: %w", err)
		}
	}
	if p.LogDir != "" {
		if err := ValidatePath(p.LogDir); err != nil {
			return nil, fmt.Errorf("invalid LogDir: %w", err)
		}
	}

	tmpl, err := template.New("plist").Parse(plistTmpl)
	if err != nil {
		return nil, fmt.Errorf("parsing plist template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, p); err != nil {
		return nil, fmt.Errorf("executing plist template: %w", err)
	}
	return buf.Bytes(), nil
}
