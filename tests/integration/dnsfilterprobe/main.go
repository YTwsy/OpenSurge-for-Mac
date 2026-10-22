package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"open-mihomo-gateway/tests/integration/dnsfilterfixture"
)

func main() {
	listen := flag.String("listen", "127.0.0.1:19100", "controlled DNS listener")
	control := flag.String("control", "", "file containing the flip-domain decision")
	flag.Parse()
	server, err := dnsfilterfixture.Start(*listen, *control)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer server.Close()
	fmt.Println("DNS_FILTER_READY", server.Address())
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()
}
