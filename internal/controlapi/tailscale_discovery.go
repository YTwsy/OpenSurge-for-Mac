package controlapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	tailscaleDiscoveryTimeout = 3 * time.Second
	tailscaleStatusLimit      = 4 << 20
)

type localTailscaleStatus struct {
	BackendState   string `json:"BackendState"`
	MagicDNSSuffix string `json:"MagicDNSSuffix"`
	CurrentTailnet struct {
		Name            string `json:"Name"`
		MagicDNSEnabled bool   `json:"MagicDNSEnabled"`
		MagicDNSSuffix  string `json:"MagicDNSSuffix"`
	} `json:"CurrentTailnet"`
	Self localTailscalePeer            `json:"Self"`
	Peer map[string]localTailscalePeer `json:"Peer"`
}

type localTailscalePeer struct {
	ID             string   `json:"ID"`
	HostName       string   `json:"HostName"`
	DNSName        string   `json:"DNSName"`
	TailscaleIPs   []string `json:"TailscaleIPs"`
	AllowedIPs     []string `json:"AllowedIPs"`
	Online         bool     `json:"Online"`
	ExitNode       bool     `json:"ExitNode"`
	ExitNodeOption bool     `json:"ExitNodeOption"`
}

func (s *Server) handleTailscaleDiscovery(w http.ResponseWriter, r *http.Request) {
	result, err := s.discoverTailscale(r.Context())
	if err != nil {
		writeJSON(w, http.StatusOK, TailscaleDiscoveryResponse{
			SchemaVersion: SchemaVersion,
			Peers:         []TailscaleDiscoveredNode{},
			Error:         compactTailscaleDiscoveryError(err),
		})
		return
	}
	result.SchemaVersion = SchemaVersion
	if result.Peers == nil {
		result.Peers = []TailscaleDiscoveredNode{}
	}
	writeJSON(w, http.StatusOK, result)
}

func discoverLocalTailscale(ctx context.Context) (TailscaleDiscoveryResponse, error) {
	binary, err := localTailscaleBinary()
	if err != nil {
		return TailscaleDiscoveryResponse{}, err
	}
	commandCtx, cancel := context.WithTimeout(ctx, tailscaleDiscoveryTimeout)
	defer cancel()
	command := localTailscaleStatusCommand(commandCtx, binary)
	var stderr bytes.Buffer
	command.Stderr = &stderr
	stdout, err := command.StdoutPipe()
	if err != nil {
		return TailscaleDiscoveryResponse{}, fmt.Errorf("prepare local Tailscale status: %w", err)
	}
	if err := command.Start(); err != nil {
		return TailscaleDiscoveryResponse{}, fmt.Errorf("start local Tailscale status: %w", err)
	}
	output, readErr := io.ReadAll(io.LimitReader(stdout, tailscaleStatusLimit+1))
	if len(output) > tailscaleStatusLimit {
		_ = command.Process.Kill()
		_ = command.Wait()
		return TailscaleDiscoveryResponse{}, fmt.Errorf("local Tailscale status exceeded %d bytes", tailscaleStatusLimit)
	}
	waitErr := command.Wait()
	if commandCtx.Err() != nil {
		return TailscaleDiscoveryResponse{}, fmt.Errorf("local Tailscale status timed out")
	}
	if readErr != nil {
		return TailscaleDiscoveryResponse{}, fmt.Errorf("read local Tailscale status: %w", readErr)
	}
	if waitErr != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = waitErr.Error()
		}
		return TailscaleDiscoveryResponse{}, fmt.Errorf("local Tailscale status failed: %s", message)
	}
	return parseLocalTailscaleStatus(output)
}

func localTailscaleStatusCommand(ctx context.Context, binary string) *exec.Cmd {
	command := exec.CommandContext(ctx, binary, "status", "--json")
	// The macOS app bundle decides between GUI and CLI mode from its launch
	// environment. LaunchAgents do not normally have TERM, so force the
	// documented CLI mode instead of letting Tailscale print a GUI error to
	// stdout where JSON is expected.
	command.Env = append(command.Environ(), "TAILSCALE_BE_CLI=1")
	return command
}

func localTailscaleBinary() (string, error) {
	paths := []string{
		"/Applications/Tailscale.app/Contents/MacOS/Tailscale",
		"/opt/homebrew/bin/tailscale",
		"/usr/local/bin/tailscale",
	}
	if home, err := os.UserHomeDir(); err == nil {
		paths = append(paths, filepath.Join(home, "Applications", "Tailscale.app", "Contents", "MacOS", "Tailscale"))
	}
	for _, path := range paths {
		info, err := os.Stat(path)
		if err == nil && info.Mode().IsRegular() && info.Mode().Perm()&0o111 != 0 {
			return path, nil
		}
	}
	return "", fmt.Errorf("local Tailscale CLI was not found")
}

func parseLocalTailscaleStatus(data []byte) (TailscaleDiscoveryResponse, error) {
	var status localTailscaleStatus
	if err := json.Unmarshal(data, &status); err != nil {
		return TailscaleDiscoveryResponse{}, fmt.Errorf("parse local Tailscale status: %w", err)
	}
	suffix := strings.TrimSuffix(strings.TrimSpace(status.CurrentTailnet.MagicDNSSuffix), ".")
	if suffix == "" {
		suffix = strings.TrimSuffix(strings.TrimSpace(status.MagicDNSSuffix), ".")
	}
	result := TailscaleDiscoveryResponse{
		Available:      true,
		BackendState:   strings.TrimSpace(status.BackendState),
		TailnetName:    strings.TrimSpace(status.CurrentTailnet.Name),
		MagicDNS:       status.CurrentTailnet.MagicDNSEnabled,
		MagicDNSSuffix: suffix,
		Peers:          make([]TailscaleDiscoveredNode, 0, len(status.Peer)),
	}
	if status.Self.ID != "" || status.Self.HostName != "" || len(status.Self.TailscaleIPs) > 0 {
		self := discoveredTailscaleNode(status.Self)
		result.Self = &self
	}
	for _, peer := range status.Peer {
		result.Peers = append(result.Peers, discoveredTailscaleNode(peer))
	}
	sort.Slice(result.Peers, func(i, j int) bool {
		if result.Peers[i].Online != result.Peers[j].Online {
			return result.Peers[i].Online
		}
		return strings.ToLower(result.Peers[i].Name) < strings.ToLower(result.Peers[j].Name)
	})
	return result, nil
}

func discoveredTailscaleNode(peer localTailscalePeer) TailscaleDiscoveredNode {
	name := strings.TrimSpace(peer.HostName)
	dnsName := strings.TrimSuffix(strings.TrimSpace(peer.DNSName), ".")
	if name == "" {
		name = strings.TrimSuffix(strings.SplitN(dnsName, ".", 2)[0], ".")
	}
	if name == "" {
		name = "Tailscale peer"
	}
	return TailscaleDiscoveredNode{
		ID:             peer.ID,
		Name:           name,
		DNSName:        dnsName,
		TailscaleIPs:   normalizedTailscaleIPs(peer.TailscaleIPs),
		Online:         peer.Online,
		ExitNode:       peer.ExitNode,
		ExitNodeOption: peer.ExitNodeOption,
		SubnetRoutes:   discoveredSubnetRoutes(peer.TailscaleIPs, peer.AllowedIPs),
	}
}

func normalizedTailscaleIPs(values []string) []string {
	out := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		address, err := netip.ParseAddr(strings.TrimSpace(value))
		if err != nil || seen[address.String()] {
			continue
		}
		seen[address.String()] = true
		out = append(out, address.String())
	}
	sort.Slice(out, func(i, j int) bool {
		left, _ := netip.ParseAddr(out[i])
		right, _ := netip.ParseAddr(out[j])
		return left.Compare(right) < 0
	})
	return out
}

func discoveredSubnetRoutes(tailscaleIPs, allowedIPs []string) []string {
	hosts := map[netip.Addr]bool{}
	for _, value := range tailscaleIPs {
		if address, err := netip.ParseAddr(strings.TrimSpace(value)); err == nil {
			hosts[address] = true
		}
	}
	seen := map[string]bool{}
	out := []string{}
	for _, value := range allowedIPs {
		prefix, err := netip.ParsePrefix(strings.TrimSpace(value))
		if err != nil {
			continue
		}
		prefix = prefix.Masked()
		if prefix.Bits() == prefix.Addr().BitLen() && hosts[prefix.Addr()] {
			continue
		}
		if !prefix.Addr().IsPrivate() || seen[prefix.String()] {
			continue
		}
		seen[prefix.String()] = true
		out = append(out, prefix.String())
	}
	sort.Strings(out)
	return out
}

func compactTailscaleDiscoveryError(err error) string {
	message := strings.Join(strings.Fields(err.Error()), " ")
	if len(message) > 240 {
		message = message[:240] + "…"
	}
	return message
}
