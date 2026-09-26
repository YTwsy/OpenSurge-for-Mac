package macosnetwork

import (
	"context"
	"fmt"
	"net/netip"
	"os/exec"
	"regexp"
	"slices"
	"strings"
	"time"

	"open-mihomo-gateway/internal/config"
	"open-mihomo-gateway/internal/runtime"
)

// SystemDNS owns only DNS servers on the configured upstream service. Search
// domains, supplemental resolvers, mDNS, other services and proxy settings are
// not written. Every write is compare-and-set/restore against a durable snapshot.
type SystemDNS struct{}

var runSCQuery = func(ctx context.Context, query string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "/usr/sbin/scutil")
	cmd.Stdin = strings.NewReader(query + "\nquit\n")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("read SystemConfiguration: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return string(output), nil
}

var serviceIDPattern = regexp.MustCompile(`Setup:/Network/Service/([A-Fa-f0-9-]{36})(?:\s|$)`)

func (SystemDNS) Prepare(ctx context.Context, iface string) (runtime.SystemDNSSnapshot, error) {
	service, err := NetworkServiceForInterface(ctx, iface)
	if err != nil {
		return runtime.SystemDNSSnapshot{}, err
	}
	listed, err := runSCQuery(ctx, "list Setup:/Network/Service/[^/]*$")
	if err != nil {
		return runtime.SystemDNSSnapshot{}, err
	}
	snapshot := runtime.SystemDNSSnapshot{NetworkService: service, Interface: iface}
	for _, match := range serviceIDPattern.FindAllStringSubmatch(listed, -1) {
		name, device, err := readDNSService(ctx, match[1])
		if err != nil {
			return runtime.SystemDNSSnapshot{}, err
		}
		if name == service && device == iface {
			if snapshot.ServiceID != "" {
				return runtime.SystemDNSSnapshot{}, fmt.Errorf("ambiguous DNS service identity for %s", service)
			}
			snapshot.ServiceID = match[1]
		}
	}
	if snapshot.ServiceID == "" {
		return runtime.SystemDNSSnapshot{}, fmt.Errorf("cannot determine stable DNS service identity for %s", service)
	}
	snapshot.Servers, err = readSystemDNS(ctx, service)
	if err != nil {
		return runtime.SystemDNSSnapshot{}, err
	}
	stateDNS, err := runSCQuery(ctx, "show State:/Network/Service/"+snapshot.ServiceID+"/DNS")
	if err != nil {
		return runtime.SystemDNSSnapshot{}, err
	}
	setupDNS, err := runSCQuery(ctx, "show Setup:/Network/Service/"+snapshot.ServiceID+"/DNS")
	if err != nil {
		return runtime.SystemDNSSnapshot{}, err
	}
	snapshot.Resolvers = slices.Clone(snapshot.Servers)
	if len(snapshot.Resolvers) == 0 {
		for _, value := range scArray(stateDNS, "ServerAddresses") {
			if _, err := netip.ParseAddr(value); err != nil {
				return runtime.SystemDNSSnapshot{}, fmt.Errorf("invalid discovered DNS address %q", value)
			}
			snapshot.Resolvers = append(snapshot.Resolvers, value)
		}
	}
	for _, data := range []string{setupDNS, stateDNS} {
		domains := append(scArray(data, "SearchDomains"), scScalar(data, "DomainName"))
		for _, domain := range domains {
			domain = strings.Trim(strings.ToLower(domain), ". ")
			if domain != "" && !slices.Contains(snapshot.Domains, domain) {
				snapshot.Domains = append(snapshot.Domains, domain)
			}
		}
	}
	return snapshot, nil
}

func (SystemDNS) Enable(ctx context.Context, snapshot runtime.SystemDNSSnapshot) error {
	service, err := verifyDNSService(ctx, snapshot)
	if err != nil {
		return err
	}
	current, err := readSystemDNS(ctx, service)
	if err != nil {
		return err
	}
	managed := []string{config.LocalSystemDNSServer}
	if !slices.Equal(current, snapshot.Servers) && !slices.Equal(current, managed) {
		return fmt.Errorf("DNS for %s changed since preparation; refusing to overwrite it", service)
	}
	return writeAndVerifyDNS(ctx, service, managed)
}

// Restore returns true when ownership was retained (including a completed
// previous restore). A foreign value releases ownership without changing DNS.
func (SystemDNS) Restore(ctx context.Context, snapshot runtime.SystemDNSSnapshot) (bool, error) {
	if !snapshot.Owned {
		return false, nil
	}
	service, err := verifyDNSService(ctx, snapshot)
	if err != nil {
		return false, err
	}
	current, err := readSystemDNS(ctx, service)
	if err != nil {
		return false, err
	}
	if slices.Equal(current, snapshot.Servers) {
		return true, nil
	}
	if !slices.Equal(current, []string{config.LocalSystemDNSServer}) {
		return false, nil
	}
	return true, writeAndVerifyDNS(ctx, service, snapshot.Servers)
}

func (SystemDNS) Verify(ctx context.Context, snapshot runtime.SystemDNSSnapshot) error {
	service, err := verifyDNSService(ctx, snapshot)
	if err != nil {
		return err
	}
	servers, err := readSystemDNS(ctx, service)
	if err != nil {
		return err
	}
	if !slices.Equal(servers, []string{config.LocalSystemDNSServer}) {
		return fmt.Errorf("DNS for %s is no longer managed by OpenSurge", service)
	}
	return nil
}

func readDNSService(ctx context.Context, id string) (string, string, error) {
	// IDs are read from SystemConfiguration or persisted in root-owned state;
	// validate them before using scutil's line-oriented command language.
	if !regexp.MustCompile(`^[A-Fa-f0-9-]{36}$`).MatchString(id) {
		return "", "", fmt.Errorf("invalid DNS service ID")
	}
	output, err := runSCQuery(ctx, "show Setup:/Network/Service/"+id+"\nshow Setup:/Network/Service/"+id+"/Interface")
	return scScalar(output, "UserDefinedName"), scScalar(output, "DeviceName"), err
}

func verifyDNSService(ctx context.Context, snapshot runtime.SystemDNSSnapshot) (string, error) {
	name, iface, err := readDNSService(ctx, snapshot.ServiceID)
	if err != nil {
		return "", err
	}
	if name == "" || iface != snapshot.Interface {
		return "", fmt.Errorf("DNS service %s was removed or changed interface; snapshot retained for recovery", snapshot.ServiceID)
	}
	return name, nil // A renamed service retains the same UUID and ownership.
}

func readSystemDNS(ctx context.Context, service string) ([]string, error) {
	output, err := runCommand(ctx, "/usr/sbin/networksetup", "-getdnsservers", service)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(output) == "There aren't any DNS Servers set on "+service+"." {
		return nil, nil
	}
	var servers []string
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		line = strings.TrimSpace(line)
		if _, err := netip.ParseAddr(line); err != nil {
			return nil, fmt.Errorf("unrecognized DNS settings for %s: %q", service, line)
		}
		servers = append(servers, line)
	}
	return servers, nil
}

func writeAndVerifyDNS(ctx context.Context, service string, servers []string) error {
	args := append([]string{"-setdnsservers", service}, servers...)
	if len(servers) == 0 {
		args = append(args, "Empty")
	}
	if _, err := runCommand(ctx, "/usr/sbin/networksetup", args...); err != nil {
		return err
	}
	actual, err := readSystemDNS(ctx, service)
	if err != nil {
		return err
	}
	if !slices.Equal(actual, servers) {
		return fmt.Errorf("DNS readback differs after setting %s", service)
	}
	return nil
}

func scScalar(output, key string) string {
	for _, line := range strings.Split(output, "\n") {
		name, value, found := strings.Cut(strings.TrimSpace(line), " : ")
		if found && name == key {
			return value
		}
	}
	return ""
}

func scArray(output, key string) []string {
	var values []string
	inArray := false
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == key+" : <array> {" {
			inArray = true
			continue
		}
		if inArray && line == "}" {
			break
		}
		if inArray {
			_, value, found := strings.Cut(line, " : ")
			if found {
				values = append(values, value)
			}
		}
	}
	return values
}
