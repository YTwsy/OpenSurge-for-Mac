package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"open-mihomo-gateway/internal/controlapi"
)

func main() {
	socket := flag.String("socket", "/var/run/opensurge/helper.sock", "Unix socket path")
	allowedRoot := flag.String("allowed-config-root", "/Library/Application Support/OpenSurge", "only configs below this directory are accepted")
	socketGroup := flag.String("socket-group", "admin", "local group allowed to connect to the fixed-function helper")
	flag.Parse()
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if err := controlapi.ServeHelper(ctx, *socket, *allowedRoot, *socketGroup); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
