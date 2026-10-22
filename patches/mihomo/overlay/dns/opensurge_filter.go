package dns

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"strings"
	"time"

	"github.com/metacubex/mihomo/component/dialer"
	icontext "github.com/metacubex/mihomo/context"
	D "github.com/miekg/dns"
	"golang.org/x/sync/singleflight"
)

// OpenSurgeFilterConfig is gateway-owned, never inherited from a subscription.
// It checks client DNS before hosts, Fake-IP and the ordinary resolver cache.
// Internal resolver calls deliberately do not enter this middleware.
type OpenSurgeFilterConfig struct {
	Enabled          bool     `yaml:"enabled" json:"enabled"`
	Server           string   `yaml:"server" json:"server"`
	Interface        string   `yaml:"interface,omitempty" json:"interface,omitempty"`
	TimeoutMS        int      `yaml:"timeout-ms" json:"timeout-ms"`
	BlockedAddresses []string `yaml:"blocked-addresses" json:"blocked-addresses"`
	BypassDomains    []string `yaml:"bypass-domains" json:"bypass-domains"`
	ProbeName        string   `yaml:"probe-name,omitempty" json:"probe-name,omitempty"`
}

func (c OpenSurgeFilterConfig) Validate() error {
	if !c.Enabled {
		return nil
	}
	ap, err := netip.ParseAddrPort(c.Server)
	if err != nil || ap.Port() == 0 || ap.Addr().Zone() != "" || ap.Addr().IsUnspecified() || ap.Addr().IsMulticast() {
		return fmt.Errorf("opensurge-external-filter.server must be a literal IP:port")
	}
	if c.TimeoutMS < 100 || c.TimeoutMS > 10000 {
		return fmt.Errorf("opensurge-external-filter.timeout-ms must be between 100 and 10000")
	}
	for _, value := range c.BlockedAddresses {
		if _, err := netip.ParseAddr(value); err != nil {
			return fmt.Errorf("invalid opensurge-external-filter blocked address")
		}
	}
	for _, value := range c.BypassDomains {
		if value == "" || strings.ContainsAny(value, "*+/#: ") {
			return fmt.Errorf("invalid opensurge-external-filter bypass domain")
		}
	}
	return nil
}

func (c OpenSurgeFilterConfig) ValidateListener(listener string) error {
	if err := c.Validate(); err != nil {
		return err
	}
	if !c.Enabled {
		return nil
	}
	endpoint, _ := netip.ParseAddrPort(c.Server)
	_, port, err := net.SplitHostPort(listener)
	if err != nil || port != fmt.Sprint(endpoint.Port()) {
		return nil
	}
	local := endpoint.Addr().IsLoopback()
	addresses, err := net.InterfaceAddrs()
	if err != nil {
		return err
	}
	for _, address := range addresses {
		prefix, err := netip.ParsePrefix(address.String())
		if err == nil && prefix.Addr().Unmap() == endpoint.Addr().Unmap() {
			local = true
		}
	}
	if local {
		return fmt.Errorf("opensurge-external-filter.server points to the local DNS listener")
	}
	return nil
}

type openSurgeFilter struct {
	config  OpenSurgeFilterConfig
	blocked map[netip.Addr]bool
	queries singleflight.Group
	slots   chan struct{}
}

func withOpenSurgeFilter(cfg OpenSurgeFilterConfig) middleware {
	f := &openSurgeFilter{config: cfg, blocked: map[netip.Addr]bool{
		netip.IPv4Unspecified(): true, netip.IPv6Unspecified(): true,
	}, slots: make(chan struct{}, 64)}
	for _, address := range cfg.BlockedAddresses {
		ip, _ := netip.ParseAddr(address)
		f.blocked[ip.Unmap()] = true
	}
	return func(next handler) handler {
		return func(ctx *icontext.DNSContext, request *D.Msg) (*D.Msg, error) {
			if len(request.Question) != 1 || request.Opcode != D.OpcodeQuery || request.Response {
				return new(D.Msg).SetRcode(request, D.RcodeFormatError), nil
			}
			q := request.Question[0]
			if q.Qclass != D.ClassINET {
				return new(D.Msg).SetRcode(request, D.RcodeRefused), nil
			}
			name := strings.TrimSuffix(strings.ToLower(q.Name), ".")
			if f.bypass(name, q.Qtype) {
				return next(ctx, request)
			}
			// Rebuild the question: never forward client identity/ECS, cookies,
			// arbitrary additional records or client-supplied answers.
			query := new(D.Msg).SetQuestion(q.Name, q.Qtype)
			query.CheckingDisabled = request.CheckingDisabled
			do := request.IsEdns0() != nil && request.IsEdns0().Do()
			query.SetEdns0(1232, do)
			key := fmt.Sprintf("%s/%d/%t/%t", name, q.Qtype, do, request.CheckingDisabled)
			// Coalesce only concurrent questions, never cache decisions. A filter
			// forwarding the same question back here waits for this finite check
			// and fails closed instead of spawning an unbounded recursive chain.
			result := f.queries.DoChan(key, func() (any, error) {
				select {
				case f.slots <- struct{}{}:
					defer func() { <-f.slots }()
				default:
					return nil, fmt.Errorf("external DNS filter is busy")
				}
				checkCtx, cancel := context.WithTimeout(context.Background(), time.Duration(cfg.TimeoutMS)*time.Millisecond)
				defer cancel()
				return f.exchange(checkCtx, query)
			})
			select {
			case <-ctx.Done():
				return new(D.Msg).SetRcode(request, D.RcodeServerFailure), nil
			case checked := <-result:
				if checked.Err != nil {
					return new(D.Msg).SetRcode(request, D.RcodeServerFailure), nil
				}
				response := checked.Val.(*D.Msg)
				if f.stop(response) {
					response = response.Copy()
					response.Id, response.Question = request.Id, request.Question
					// This is a per-hop result, not authenticated DNSSEC data.
					response.AuthenticatedData = false
					response.Extra = nil
					ctx.SetType("external-filter")
					return response, nil
				}
				return next(ctx, request)
			}
		}
	}
}

func (f *openSurgeFilter) bypass(name string, qtype uint16) bool {
	if qtype == D.TypeTXT && f.config.ProbeName != "" && name == f.config.ProbeName {
		return true
	}
	// Local namespaces stay with existing hosts/lease/resolver behavior.
	if !strings.Contains(name, ".") {
		return true
	}
	for _, suffix := range append([]string{"local", "localhost", "lan", "home.arpa", "in-addr.arpa", "ip6.arpa"}, f.config.BypassDomains...) {
		suffix = strings.TrimSuffix(strings.ToLower(suffix), ".")
		if name == suffix || strings.HasSuffix(name, "."+suffix) {
			return true
		}
	}
	return false
}

func (f *openSurgeFilter) stop(response *D.Msg) bool {
	if response.Rcode != D.RcodeSuccess || len(response.Answer) == 0 {
		return true
	}
	for _, rr := range response.Answer {
		var ip net.IP
		switch v := rr.(type) {
		case *D.A:
			ip = v.A
		case *D.AAAA:
			ip = v.AAAA
		}
		if address, ok := netip.AddrFromSlice(ip); ok && f.blocked[address.Unmap()] {
			return true
		}
	}
	return false
}

func (f *openSurgeFilter) exchange(ctx context.Context, query *D.Msg) (*D.Msg, error) {
	response, err := f.exchangeTransport(ctx, "udp", query)
	if err == nil && response.Truncated {
		response, err = f.exchangeTransport(ctx, "tcp", query)
	}
	if err != nil {
		return nil, err
	}
	if !response.Response || response.Opcode != D.OpcodeQuery || response.Truncated || len(response.Question) != 1 ||
		!strings.EqualFold(response.Question[0].Name, query.Question[0].Name) ||
		response.Question[0].Qtype != query.Question[0].Qtype || response.Question[0].Qclass != D.ClassINET {
		return nil, fmt.Errorf("invalid external DNS filter response")
	}
	return response, nil
}

func (f *openSurgeFilter) exchangeTransport(ctx context.Context, network string, query *D.Msg) (*D.Msg, error) {
	// Literal address + the engine's direct socket dialer avoid bootstrap DNS,
	// proxy selection, respect-rules and re-entry through the system TUN.
	conn, err := dialer.DialContext(ctx, network, f.config.Server, dialer.WithInterface(f.config.Interface))
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()
	client := &D.Client{Net: network, UDPSize: 1232, Timeout: time.Duration(f.config.TimeoutMS) * time.Millisecond}
	response, _, err := client.ExchangeWithConnContext(ctx, query, &D.Conn{Conn: conn, UDPSize: 1232})
	return response, err
}
