package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"open-mihomo-gateway/internal/config"
	"open-mihomo-gateway/internal/device"
	"open-mihomo-gateway/internal/doctor"
	"open-mihomo-gateway/internal/gateway"
	"open-mihomo-gateway/internal/runtime"
)

const defaultConfigPath = "examples/config.example.yaml"

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	if len(args) == 0 {
		printUsage(os.Stderr)
		return 2
	}

	command := args[0]
	fs := flag.NewFlagSet(command, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	configPath := fs.String("config", defaultConfigPath, "path to gateway config")
	if err := fs.Parse(args[1:]); err != nil {
		return 2
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "config: %v\n", err)
		return 1
	}

	ctx := context.Background()
	manager := gateway.New(cfg)

	switch command {
	case "start":
		if err := manager.Start(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "start: %v\n", err)
			return 1
		}
	case "stop":
		if err := manager.Stop(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "stop: %v\n", err)
			return 1
		}
	case "status":
		status, err := manager.Status(ctx)
		if err != nil {
			fmt.Fprintf(os.Stderr, "status: %v\n", err)
			return 1
		}
		fmt.Print(status.Format())
	case "doctor":
		report := doctor.Run(cfg)
		fmt.Print(report.Format())
	case "leases":
		clients, err := device.LoadLeases(runtime.NewPaths(cfg).LeaseFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "leases: %v\n", err)
			return 1
		}
		fmt.Print(device.FormatClients(clients))
	case "logs":
		fmt.Printf("Logs directory: %s\n", runtime.NewPaths(cfg).LogDir)
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n", command)
		printUsage(os.Stderr)
		return 2
	}

	return 0
}

func printUsage(out *os.File) {
	fmt.Fprintf(out, `Open Mihomo Gateway for macOS

Usage:
  omg <command> --config <path>

Commands:
  start    prepare runtime state and start gateway services
  stop     stop gateway services and clean runtime state
  status   print gateway status
  doctor   run environment checks
  leases   print DHCP leases
  logs     print runtime log location

Default config: %s
`, defaultConfigPath)
}
