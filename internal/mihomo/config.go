package mihomo

import (
	"bytes"
	"text/template"

	"open-mihomo-gateway/internal/config"
)

const configTemplate = `mixed-port: {{ .MixedPort }}
allow-lan: true
bind-address: "*"
mode: rule
log-level: info
{{ if .TUNEnabled }}
interface-name: {{ .UpstreamInterface }}
{{ end }}

external-controller: {{ .APIAddr }}
{{- if .Secret }}
secret: {{ .Secret }}
{{- end }}

profile:
  store-selected: true

dns:
  enable: true
  listen: 0.0.0.0:1053
  ipv6: false
  enhanced-mode: fake-ip
  fake-ip-range: 198.18.0.1/16
  nameserver:
    - 1.1.1.1
    - 8.8.8.8

{{ if .TUNEnabled }}
tun:
  enable: true
  stack: {{ .TUNStack }}
  device: {{ .TUNDevice }}
  auto-route: {{ .TUNAutoRoute }}
  auto-detect-interface: {{ .TUNAutoDetectInterface }}
  strict-route: {{ .TUNStrictRoute }}
  dns-hijack:
    - any:53
  route-exclude-address:
    - {{ .LANPrefix }}
    - 127.0.0.0/8
    - 10.0.0.0/8
    - 172.16.0.0/12
    - 192.168.0.0/16
    - 224.0.0.0/4
    - 255.255.255.255/32

{{ end }}
{{ .PolicySections }}
`

func RenderConfig(cfg config.Config) (string, error) {
	tmpl, err := template.New("mihomo").Parse(configTemplate)
	if err != nil {
		return "", err
	}
	data, err := newTemplateData(cfg)
	if err != nil {
		return "", err
	}
	var out bytes.Buffer
	if err := tmpl.Execute(&out, data); err != nil {
		return "", err
	}
	return out.String(), nil
}

type templateData struct {
	config.MihomoConfig
	TUNEnabled             bool
	TUNDevice              string
	TUNStack               string
	TUNAutoRoute           bool
	TUNAutoDetectInterface bool
	TUNStrictRoute         bool
	UpstreamInterface      string
	LANPrefix              string
	UpstreamProxy          config.UpstreamProxyConfig
	PolicySections         string
}

func newTemplateData(cfg config.Config) (templateData, error) {
	lanPrefix, err := cfg.LANPrefix24()
	if err != nil {
		return templateData{}, err
	}
	policySections, err := renderPolicySections(cfg)
	if err != nil {
		return templateData{}, err
	}
	transparent := cfg.Transparent
	return templateData{
		MihomoConfig:           cfg.Mihomo,
		TUNEnabled:             transparent.TUNEnabled(),
		TUNDevice:              transparent.TUNDevice,
		TUNStack:               transparent.TUNStack,
		TUNAutoRoute:           transparent.TUNAutoRoute,
		TUNAutoDetectInterface: transparent.TUNAutoDetectInterface,
		TUNStrictRoute:         transparent.TUNStrictRoute,
		UpstreamInterface:      cfg.Gateway.UpstreamInterface,
		LANPrefix:              lanPrefix,
		UpstreamProxy:          cfg.UpstreamProxy,
		PolicySections:         policySections,
	}, nil
}
