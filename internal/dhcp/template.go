package dhcp

import (
	"bytes"
	"text/template"

	"open-mihomo-gateway/internal/config"
	"open-mihomo-gateway/internal/device"
	"open-mihomo-gateway/internal/runtime"
)

const dnsmasqTemplate = `interface={{ .Interface }}
bind-interfaces

{{ if .DHCPEnabled }}
dhcp-range={{ .RangeStart }},{{ .RangeEnd }},{{ .LeaseTime }}
dhcp-option=option:router,{{ .GatewayIP }}
dhcp-option=option:dns-server,{{ .GatewayIP }}
domain={{ .Domain }}
{{ range .Reservations }}dhcp-host={{ .MAC }},{{ .IPv4 }}
{{ end }}

log-dhcp
dhcp-leasefile={{ .LeaseFile }}
{{ end }}
log-queries

pid-file={{ .PIDFile }}

port={{ .DNSPort }}
listen-address={{ .DNSListen }}
{{ if .DNSUpstream }}
no-resolv
server={{ .DNSUpstream }}
{{ end }}
`

type templateData struct {
	DHCPEnabled  bool
	Interface    string
	RangeStart   string
	RangeEnd     string
	LeaseTime    string
	GatewayIP    string
	Domain       string
	LeaseFile    string
	PIDFile      string
	DNSPort      int
	DNSListen    string
	DNSUpstream  string
	Reservations []device.Reservation
}

func RenderConfig(cfg config.Config, paths runtime.Paths) (string, error) {
	var reservations []device.Reservation
	bundle := cfg.DevicePolicy.Bundle
	if bundle == nil && cfg.DevicePolicy.File != "" {
		loaded, err := device.LoadPolicyBundle(cfg.DevicePolicy.File)
		if err != nil {
			return "", err
		}
		bundle = &loaded
	}
	if bundle != nil {
		reservations = bundle.Compiled.Reservations
	}
	data := templateData{
		DHCPEnabled:  cfg.DHCP.Enabled,
		Interface:    cfg.Gateway.Interface,
		RangeStart:   cfg.DHCP.RangeStart,
		RangeEnd:     cfg.DHCP.RangeEnd,
		LeaseTime:    cfg.DHCP.LeaseTime,
		GatewayIP:    cfg.Gateway.LANIP,
		Domain:       cfg.DHCP.Domain,
		LeaseFile:    paths.LeaseFile,
		PIDFile:      paths.DNSMasqPIDFile,
		DNSPort:      cfg.DNS.Port,
		DNSListen:    cfg.DNS.Listen,
		DNSUpstream:  cfg.DNS.Upstream,
		Reservations: reservations,
	}

	tmpl, err := template.New("dnsmasq").Parse(dnsmasqTemplate)
	if err != nil {
		return "", err
	}
	var out bytes.Buffer
	if err := tmpl.Execute(&out, data); err != nil {
		return "", err
	}
	return out.String(), nil
}
