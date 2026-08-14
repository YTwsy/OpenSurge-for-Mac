package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"open-mihomo-gateway/internal/ipv6packet"
	"open-mihomo-gateway/internal/macosnetwork"
)

var (
	discoverNetwork = macosnetwork.Discover
	probeDHCP       = macosnetwork.ProbeDHCPServers
	restoreDHCP     = macosnetwork.SetDHCP
	effectiveUID    = os.Geteuid
	runIPv6Broker   = ipv6packet.RunBroker
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	os.Exit(runContext(ctx, os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	return runContext(context.Background(), args, stdout, stderr)
}

func runContext(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: opensurge-network <discover|probe-dhcp|restore-dhcp|ipv6-packet>")
		return 2
	}
	switch args[0] {
	case "discover":
		flags := flag.NewFlagSet("discover", flag.ContinueOnError)
		flags.SetOutput(stderr)
		service := flags.String("service", "Wi-Fi", "macOS network service")
		iface := flags.String("interface", "", "BSD interface")
		if flags.Parse(args[1:]) != nil || *iface == "" {
			return 2
		}
		snapshot, err := discoverNetwork(ctx, *service, *iface)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		_ = json.NewEncoder(stdout).Encode(snapshot)
		return 0
	case "probe-dhcp":
		flags := flag.NewFlagSet("probe-dhcp", flag.ContinueOnError)
		flags.SetOutput(stderr)
		iface := flags.String("interface", "", "BSD interface")
		expect := flags.String("expect", "", "required expectation: none or any")
		timeout := flags.Duration("timeout", 3*time.Second, "probe duration")
		if flags.Parse(args[1:]) != nil || *iface == "" || (*expect != "none" && *expect != "any") {
			return 2
		}
		if effectiveUID() != 0 {
			fmt.Fprintln(stderr, "probe-dhcp requires root")
			return 1
		}
		servers, err := probeDHCP(ctx, *iface, *timeout)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		_ = json.NewEncoder(stdout).Encode(map[string]any{"interface": *iface, "servers": servers})
		if *expect == "none" && len(servers) != 0 {
			fmt.Fprintln(stderr, "unexpected DHCP OFFER detected")
			return 3
		}
		if *expect == "any" && len(servers) == 0 {
			fmt.Fprintln(stderr, "no DHCP OFFER detected")
			return 3
		}
		return 0
	case "restore-dhcp":
		flags := flag.NewFlagSet("restore-dhcp", flag.ContinueOnError)
		flags.SetOutput(stderr)
		service := flags.String("service", "Wi-Fi", "macOS network service")
		if flags.Parse(args[1:]) != nil {
			return 2
		}
		if effectiveUID() != 0 {
			fmt.Fprintln(stderr, "restore-dhcp requires root")
			return 1
		}
		if err := restoreDHCP(ctx, *service); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		fmt.Fprintf(stdout, "restored %s to DHCP and automatic DNS\n", *service)
		return 0
	case "ipv6-packet":
		flags := flag.NewFlagSet("ipv6-packet", flag.ContinueOnError)
		flags.SetOutput(stderr)
		iface := flags.String("interface", "", "downstream Ethernet interface")
		socket := flags.String("socket", "", "mihomo OpenSurge packet listener socket")
		readyFile := flags.String("ready-file", "", "readiness marker path")
		mtu := flags.Int("mtu", 1500, "IPv6 packet MTU")
		if flags.Parse(args[1:]) != nil || *iface == "" || *socket == "" || *readyFile == "" {
			return 2
		}
		if effectiveUID() != 0 {
			fmt.Fprintln(stderr, "ipv6-packet requires root")
			return 1
		}
		if err := ipv6packet.EnsureSocketParent(*socket); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		fmt.Fprintf(stdout, "OpenSurge IPv6 packet broker attaching to %s\n", *iface)
		if err := runIPv6Broker(ctx, ipv6packet.BrokerConfig{Interface: *iface, Socket: *socket, ReadyFile: *readyFile, MTU: *mtu}); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		return 0
	default:
		fmt.Fprintf(stderr, "unknown command %q\n", args[0])
		return 2
	}
}
