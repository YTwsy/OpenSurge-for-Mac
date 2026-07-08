package pf

import (
	"bytes"
	"text/template"

	"open-mihomo-gateway/internal/config"
)

const anchorTemplate = `{{ if .SameLAN }}nat on {{ .UpstreamInterface }} from {{ .LanCIDR }} to ! {{ .LanCIDR }} -> ({{ .UpstreamInterface }})
{{ else }}nat on {{ .UpstreamInterface }} from {{ .LanCIDR }} to any -> ({{ .UpstreamInterface }})
{{ end }}

pass in all
pass out all
`

type templateData struct {
	UpstreamInterface string
	LanCIDR           string
	SameLAN           bool
}

func RenderAnchor(cfg config.Config) (string, error) {
	lanCIDR, err := cfg.LANPrefix24()
	if err != nil {
		return "", err
	}
	data := templateData{
		UpstreamInterface: cfg.Gateway.UpstreamInterface,
		LanCIDR:           lanCIDR,
		SameLAN:           cfg.Gateway.SameLAN(),
	}

	tmpl, err := template.New("pf-anchor").Parse(anchorTemplate)
	if err != nil {
		return "", err
	}
	var out bytes.Buffer
	if err := tmpl.Execute(&out, data); err != nil {
		return "", err
	}
	return out.String(), nil
}
