package mihomo

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/dns/dnsmessage"
	"gopkg.in/yaml.v3"
)

type preparedDNSError struct{ err error }

func (e *preparedDNSError) Error() string { return "prepared DNS not ready: " + e.err.Error() }
func (e *preparedDNSError) Unwrap() error { return e.err }

// Hold both transports until immediately before fork. The engine cannot inherit
// these sockets, so readiness and bounded retry still handle the release/bind race.
func reservePreparedDNS() (string, func(), error) {
	for attempt := 0; attempt < 8; attempt++ {
		tcp, err := net.Listen("tcp4", "127.0.0.1:0")
		if err != nil {
			return "", nil, err
		}
		addr := tcp.Addr().String()
		udp, err := net.ListenPacket("udp4", addr)
		if err != nil {
			_ = tcp.Close()
			continue
		}
		var once sync.Once
		return addr, func() { once.Do(func() { _ = tcp.Close(); _ = udp.Close() }) }, nil
	}
	return "", nil, fmt.Errorf("could not reserve a loopback port for both UDP and TCP")
}

func preparedNeedsDNS(rendered string) (bool, error) {
	refs, err := DNSReferenceBindings(rendered)
	if err != nil {
		return false, err
	}
	return hasSelfDNSReference(refs), nil
}

func newDNSProbeName() (string, error) {
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return "", err
	}
	return "opensurge-ready-" + hex.EncodeToString(nonce[:]) + ".invalid", nil
}

func bindPreparedDNS(root *yaml.Node, rendered, address, probeName string) error {
	refs, err := DNSReferenceBindings(rendered)
	if err != nil {
		return err
	}
	needed := false
	for _, ref := range refs {
		needed = needed || ref.Service == "self"
	}
	dns := dnsValue(root, "dns")
	if dns == nil {
		if needed {
			return fmt.Errorf("self DNS binding has no DNS section")
		}
		return nil
	}
	if dns.Kind != yaml.MappingNode {
		return fmt.Errorf("prepared dns must be a mapping")
	}
	setPreparedField(dns, "listen", "")
	if !needed {
		return nil
	}
	if enabled := dnsValue(dns, "enable"); enabled == nil || enabled.Tag != "!!bool" || enabled.Value != "true" {
		return fmt.Errorf("self DNS binding requires the managed DNS resolver to be enabled")
	}
	host, port, err := net.SplitHostPort(address)
	n, portErr := strconv.Atoi(port)
	if err != nil || portErr != nil || host != "127.0.0.1" || n < 1024 || n > 65535 || probeName == "" {
		return fmt.Errorf("self DNS references require an allocated private loopback DNS endpoint and probe identity")
	}
	if !strings.HasPrefix(probeName, "opensurge-ready-") || !strings.HasSuffix(probeName, ".invalid") {
		return fmt.Errorf("invalid prepared DNS probe identity")
	}
	// All resolver aliases are expanded before per-field rebinding.
	dns, err = copyDNSNode(dns)
	if err != nil {
		return err
	}
	root.Content[mappingValueIndex(root, "dns")] = dns
	targets, err := dnsTargets(dns)
	if err != nil {
		return err
	}
	byField := map[string]dnsTarget{}
	for _, target := range targets {
		byField[target.field] = target
	}
	seen := map[string]bool{}
	for _, ref := range refs {
		if ref.Service != "self" {
			continue
		}
		target := byField[ref.Field]
		node := target.node
		parsed := parseDNSAddress(ref.Effective)
		if seen[ref.Field] || node == nil || node.Value != ref.Effective || target.purpose != ref.Purpose || !parsed.simple || parsed.host != "127.0.0.1" || parsed.port != "1053" || (ref.Purpose != "proxy-server-nameserver" && ref.Purpose != "proxy-server-nameserver-policy") {
			return fmt.Errorf("stale or invalid DNS self binding for %s", ref.Field)
		}
		seen[ref.Field] = true
		node.Value = address
		if strings.Contains(ref.Effective, "://") {
			node.Value = parsed.scheme + "://" + address
		}
	}
	setPreparedField(dns, "listen", address)
	formalProbe, err := finalDNSProbeName(rendered)
	if err != nil {
		return err
	}
	if policy := dnsValue(dns, "nameserver-policy"); policy != nil {
		if i := mappingValueIndex(policy, formalProbe); i >= 0 {
			policy.Content = append(policy.Content[:i-1], policy.Content[i+1:]...)
		}
	}
	return addDNSReadinessProbe(dns, probeName)
}

func addDNSReadinessProbe(dns *yaml.Node, probeName string) error {
	// An exact .invalid TXT policy terminates within this engine. It does
	// not depend on hosts, Fake-IP, upstream connectivity or historical caches.
	// Socket ownership below additionally prevents a different local DNS server
	// from satisfying readiness when this core lost the listener bind race.
	policy := dnsMapping(dns, "nameserver-policy")
	if policy.Kind != yaml.MappingNode {
		return fmt.Errorf("nameserver-policy must be a mapping")
	}
	if dnsValue(policy, probeName) != nil {
		return fmt.Errorf("managed DNS probe name collision")
	}
	setPreparedField(policy, probeName, "rcode://success")
	return nil
}

func probePreparedDNS(ctx context.Context, state PreparedState) error {
	if state.DNSAddr == "" {
		return nil
	}
	if err := verifyPreparedDNSSockets(ctx, state); err != nil {
		return err
	}
	for _, network := range []string{"udp", "tcp"} {
		if err := queryPreparedDNS(ctx, network, state.DNSAddr, state.DNSProbeName); err != nil {
			return fmt.Errorf("%s DNS probe: %w", network, err)
		}
	}
	return nil
}

func verifyPreparedDNSSockets(ctx context.Context, state PreparedState) error {
	// OpenSurge's host is macOS, where lsof is a system tool. A query response
	// alone cannot prove which process owns a UDP socket (Mihomo uses SO_REUSEADDR).
	output, err := exec.CommandContext(ctx, "lsof", "-nP", "-a", "-p", strconv.Itoa(state.PID), "-i", "-FfPnT").Output()
	if err != nil {
		return fmt.Errorf("verify DNS socket ownership: %w", err)
	}
	tcp, udp, protocol, name, listening := false, false, "", "", false
	finish := func() {
		if name != state.DNSAddr {
			return
		}
		if protocol == "TCP" && listening {
			tcp = true
		}
		if protocol == "UDP" {
			udp = true
		}
	}
	for _, line := range strings.Split(string(output), "\n") {
		if line == "" {
			continue
		}
		switch line[0] {
		case 'f':
			finish()
			protocol, name, listening = "", "", false
		case 'P':
			protocol = line[1:]
		case 'n':
			name = line[1:]
		case 'T':
			if line == "TST=LISTEN" {
				listening = true
			}
		}
	}
	finish()
	if !tcp || !udp {
		return fmt.Errorf("current process does not own both DNS listeners (TCP=%t UDP=%t)", tcp, udp)
	}
	return nil
}

func queryPreparedDNS(ctx context.Context, network, address, probeName string) error {
	name, err := dnsmessage.NewName(probeName + ".")
	if err != nil {
		return err
	}
	var nonce [2]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return err
	}
	id := binary.BigEndian.Uint16(nonce[:])
	question := dnsmessage.Question{Name: name, Type: dnsmessage.TypeTXT, Class: dnsmessage.ClassINET}
	request := dnsmessage.Message{Header: dnsmessage.Header{ID: id, RecursionDesired: true}, Questions: []dnsmessage.Question{question}}
	payload, err := request.Pack()
	if err != nil {
		return err
	}
	conn, err := (&net.Dialer{}).DialContext(ctx, network, address)
	if err != nil {
		return err
	}
	defer conn.Close()
	deadline := time.Now().Add(500 * time.Millisecond)
	if limit, ok := ctx.Deadline(); ok && limit.Before(deadline) {
		deadline = limit
	}
	if err := conn.SetDeadline(deadline); err != nil {
		return err
	}
	if network == "tcp" {
		payload = append([]byte{byte(len(payload) >> 8), byte(len(payload))}, payload...)
	}
	if _, err := conn.Write(payload); err != nil {
		return err
	}
	response := make([]byte, 4096)
	if network == "tcp" {
		var length [2]byte
		if _, err := io.ReadFull(conn, length[:]); err != nil {
			return err
		}
		n := int(binary.BigEndian.Uint16(length[:]))
		if n > len(response) {
			return fmt.Errorf("oversized DNS readiness response")
		}
		response = response[:n]
		if _, err := io.ReadFull(conn, response); err != nil {
			return err
		}
	} else {
		n, err := conn.Read(response)
		if err != nil {
			return err
		}
		response = response[:n]
	}
	var message dnsmessage.Message
	if err := message.Unpack(response); err != nil {
		return err
	}
	if message.ID != id || !message.Response || message.Truncated || message.RCode != dnsmessage.RCodeSuccess || len(message.Questions) != 1 || message.Questions[0] != question || len(message.Answers) != 0 {
		return fmt.Errorf("unexpected DNS readiness response")
	}
	return nil
}
