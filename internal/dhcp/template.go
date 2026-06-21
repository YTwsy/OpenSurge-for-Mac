package dhcp

import (
	"bytes"
	"text/template"

	"open-mihomo-gateway/internal/config"
	"open-mihomo-gateway/internal/runtime"
)

const dnsmasqTemplate = `interface={{ .Interface }}
bind-interfaces

dhcp-range={{ .RangeStart }},{{ .RangeEnd }},{{ .LeaseTime }}
dhcp-option=option:router,{{ .GatewayIP }}
dhcp-option=option:dns-server,{{ .GatewayIP }}
domain={{ .Domain }}

log-dhcp
log-queries

dhcp-leasefile={{ .LeaseFile }}
pid-file={{ .PIDFile }}

port={{ .DNSPort }}
listen-address={{ .DNSListen }}
`

type templateData struct {
	Interface  string
	RangeStart string
	RangeEnd   string
	LeaseTime  string
	GatewayIP  string
	Domain     string
	LeaseFile  string
	PIDFile    string
	DNSPort    int
	DNSListen  string
}

func RenderConfig(cfg config.Config, paths runtime.Paths) (string, error) {
	data := templateData{
		Interface:  cfg.Gateway.Interface,
		RangeStart: cfg.DHCP.RangeStart,
		RangeEnd:   cfg.DHCP.RangeEnd,
		LeaseTime:  cfg.DHCP.LeaseTime,
		GatewayIP:  cfg.Gateway.LANIP,
		Domain:     cfg.DHCP.Domain,
		LeaseFile:  paths.LeaseFile,
		PIDFile:    paths.DNSMasqPIDFile,
		DNSPort:    cfg.DNS.Port,
		DNSListen:  cfg.DNS.Listen,
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
