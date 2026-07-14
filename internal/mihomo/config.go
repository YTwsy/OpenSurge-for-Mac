package mihomo

import (
	"bytes"
	"strings"
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

# Use MetaCubeX's documented CDN endpoints instead of the GitHub release URLs
# baked into mihomo. Imported profiles can contain GEOIP/GEOSITE/GEOASN rules,
# and engine validation must not depend on a slow GitHub asset download.
geox-url:
  geoip: https://testingcf.jsdelivr.net/gh/MetaCubeX/meta-rules-dat@release/geoip.dat
  geosite: https://testingcf.jsdelivr.net/gh/MetaCubeX/meta-rules-dat@release/geosite.dat
  mmdb: https://testingcf.jsdelivr.net/gh/MetaCubeX/meta-rules-dat@release/geoip.metadb
  asn: https://testingcf.jsdelivr.net/gh/MetaCubeX/meta-rules-dat@release/GeoLite2-ASN.mmdb

dns:
  enable: true
  listen: 0.0.0.0:1053
  ipv6: false
  enhanced-mode: fake-ip
  fake-ip-range: 198.18.0.1/16
{{ .DNSResolverFields }}

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
	DNSResolverFields      string
	PolicySections         string
}

func newTemplateData(cfg config.Config) (templateData, error) {
	lanPrefix, err := cfg.LANPrefix24()
	if err != nil {
		return templateData{}, err
	}
	var imported *importedProfile
	dnsResolverFields := defaultDNSResolverFieldsYAML
	if cfg.Mihomo.ProfileMode == config.MihomoProfileModeImported {
		loaded, err := loadImportedProfile(cfg.Mihomo.Profile)
		if err != nil {
			return templateData{}, err
		}
		imported = &loaded
		dnsResolverFields = loaded.dnsResolverFields
	}
	policySections, err := renderPolicySections(cfg, imported)
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
		DNSResolverFields:      indentYAMLBlock(dnsResolverFields, "  "),
		PolicySections:         policySections,
	}, nil
}

func indentYAMLBlock(value, indent string) string {
	lines := strings.Split(strings.TrimRight(value, "\n"), "\n")
	for i := range lines {
		lines[i] = indent + lines[i]
	}
	return strings.Join(lines, "\n")
}
