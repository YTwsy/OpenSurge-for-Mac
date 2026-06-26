package pf

import (
	"bytes"
	"text/template"

	"open-mihomo-gateway/internal/config"
)

const anchorTemplate = `nat on {{ .UpstreamInterface }} from {{ .LanCIDR }} to any -> ({{ .UpstreamInterface }})

{{ if gt .RedirPort 0 }}
rdr pass on {{ .LanInterface }} proto tcp from {{ .LanCIDR }} to any -> 127.0.0.1 port {{ .RedirPort }}
{{ end }}

pass in all
pass out all
`

type templateData struct {
	UpstreamInterface string
	LanInterface      string
	LanCIDR           string
	RedirPort         int
}

func RenderAnchor(cfg config.Config) (string, error) {
	lanCIDR, err := cfg.LANPrefix24()
	if err != nil {
		return "", err
	}
	data := templateData{
		UpstreamInterface: cfg.Gateway.UpstreamInterface,
		LanInterface:      cfg.Gateway.Interface,
		LanCIDR:           lanCIDR,
		RedirPort:         cfg.PF.RedirectTCPTo,
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
