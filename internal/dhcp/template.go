package dhcp

import (
	"bytes"
	"net"
	"strings"
	"text/template"

	"open-mihomo-gateway/internal/config"
	"open-mihomo-gateway/internal/device"
	"open-mihomo-gateway/internal/runtime"
)

const dnsmasqTemplate = `interface={{ .Interface }}
bind-interfaces

{{ if .DHCPEnabled }}
dhcp-range={{ .RangeStart }},{{ .RangeEnd }},{{ .Netmask }},{{ .LeaseTime }}
dhcp-option=option:router,{{ .GatewayIP }}
dhcp-option=option:dns-server,{{ .GatewayIP }}
{{ if .RouterBypassEnabled }}dhcp-option=tag:opensurge-router-bypass,option:router,{{ .BypassGateway }}
dhcp-option=tag:opensurge-router-bypass,option:dns-server,{{ .BypassDNS }}
{{ end }}
domain={{ .Domain }}
{{ range .Reservations }}dhcp-host={{ .MAC }}{{ if eq .GatewayTarget "upstream_router" }},set:opensurge-router-bypass{{ end }},{{ .IPv4 }}
{{ end }}
{{ if .IPv6RAEnabled }}
enable-ra
dhcp-range={{ .IPv6Prefix }},ra-stateless,64,{{ .LeaseTime }}
dhcp-option=option6:dns-server,[fe80::]
ra-param={{ .Interface }},20,60
{{ end }}

log-dhcp
dhcp-leasefile={{ .LeaseFile }}
{{ end }}
log-queries

pid-file={{ .PIDFile }}

port={{ .DNSPort }}
listen-address={{ .DNSListen }}
{{ if .IPv6GatewayEnabled }}listen-address={{ .IPv6Gateway }}
{{ end }}
{{ if .DNSUpstream }}
no-resolv
server={{ .DNSUpstream }}
{{ end }}
`

type templateData struct {
	DHCPEnabled         bool
	Interface           string
	RangeStart          string
	RangeEnd            string
	Netmask             string
	LeaseTime           string
	GatewayIP           string
	BypassGateway       string
	BypassDNS           string
	RouterBypassEnabled bool
	Domain              string
	LeaseFile           string
	PIDFile             string
	DNSPort             int
	DNSListen           string
	DNSUpstream         string
	Reservations        []device.Reservation
	IPv6GatewayEnabled  bool
	IPv6RAEnabled       bool
	IPv6Gateway         string
	IPv6Prefix          string
}

func RenderConfig(cfg config.Config, paths runtime.Paths) (string, error) {
	scope, err := cfg.LANScope()
	if err != nil {
		return "", err
	}
	var reservations []device.Reservation
	bundle := cfg.DevicePolicy.Bundle
	if bundle == nil && cfg.DevicePolicy.File != "" {
		loaded, err := device.LoadPolicyBundleForIPOnlyMode(cfg.DevicePolicy.File, cfg.Gateway.Mode == config.GatewayModeSameLAN)
		if err != nil {
			return "", err
		}
		bundle = &loaded
	}
	if bundle != nil {
		// dnsmasq refuses to start on a dhcp-host outside the served subnet, so
		// registrations left over from another LAN are skipped instead of
		// taking the whole gateway down with them.
		for _, reservation := range bundle.Compiled.Reservations {
			if scope.Contains(net.ParseIP(reservation.IPv4)) {
				reservations = append(reservations, reservation)
			}
		}
	}
	routerBypassEnabled := false
	for _, reservation := range reservations {
		if reservation.GatewayTarget == device.GatewayTargetUpstreamRouter {
			routerBypassEnabled = true
			break
		}
	}
	dnsUpstream := strings.TrimSpace(cfg.DNS.Upstream)
	if dnsUpstream == "" {
		dnsUpstream = config.MihomoDNSUpstream
	}
	data := templateData{
		DHCPEnabled:         cfg.DHCP.Enabled,
		Interface:           cfg.Gateway.Interface,
		RangeStart:          cfg.DHCP.RangeStart,
		RangeEnd:            cfg.DHCP.RangeEnd,
		Netmask:             net.IP(scope.Network.Mask).String(),
		LeaseTime:           cfg.DHCP.LeaseTime,
		GatewayIP:           cfg.Gateway.LANIP,
		BypassGateway:       cfg.DHCP.BypassGateway,
		BypassDNS:           strings.Join(cfg.DHCP.BypassDNS, ","),
		RouterBypassEnabled: routerBypassEnabled,
		Domain:              cfg.DHCP.Domain,
		LeaseFile:           paths.LeaseFile,
		PIDFile:             paths.DNSMasqPIDFile,
		DNSPort:             cfg.DNS.Port,
		DNSListen:           cfg.DNS.Listen,
		DNSUpstream:         dnsUpstream,
		Reservations:        reservations,
		IPv6GatewayEnabled:  cfg.Transparent.TUNIPv6 != config.TUNIPv6Off,
		// same_lan is selective, manual IPv6 onboarding. Advertising RA on a
		// shared LAN would silently move devices that were never selected for
		// the bypass-router path. DHCP-owning topologies deliberately remain
		// LAN-wide providers.
		IPv6RAEnabled: cfg.Transparent.TUNIPv6 != config.TUNIPv6Off && cfg.DHCP.Enabled,
		IPv6Gateway:   config.DownstreamIPv6Gateway,
		IPv6Prefix:    strings.TrimSuffix(config.DownstreamIPv6Prefix, "/64"),
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
