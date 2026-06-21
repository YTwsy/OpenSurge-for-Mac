package mihomo

import (
	"bytes"
	"text/template"

	"open-mihomo-gateway/internal/config"
)

const configTemplate = `mixed-port: {{ .MixedPort }}
redir-port: {{ .RedirPort }}
allow-lan: true
bind-address: "*"
mode: rule
log-level: info

external-controller: {{ .APIAddr }}
{{- if .Secret }}
secret: {{ .Secret }}
{{- end }}

dns:
  enable: true
  listen: 0.0.0.0:1053
  ipv6: false
  enhanced-mode: fake-ip
  fake-ip-range: 198.18.0.1/16
  nameserver:
    - 1.1.1.1
    - 8.8.8.8

proxies: []

proxy-groups:
  - name: DIRECT
    type: select
    proxies:
      - DIRECT

rules:
  - MATCH,DIRECT
`

func RenderConfig(cfg config.Config) (string, error) {
	tmpl, err := template.New("mihomo").Parse(configTemplate)
	if err != nil {
		return "", err
	}
	var out bytes.Buffer
	if err := tmpl.Execute(&out, cfg.Mihomo); err != nil {
		return "", err
	}
	return out.String(), nil
}
