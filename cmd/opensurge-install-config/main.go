package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"open-mihomo-gateway/internal/installconfig"
)

func main() {
	source := flag.String("source", "", "source gateway configuration")
	root := flag.String("root", "/Library/Application Support/OpenSurge", "installed root-owned data directory")
	output := flag.String("output", "", "destination config; defaults to <root>/config.yaml")
	validatePackageSource := flag.Bool("validate-package-source", false, "validate that a config is self-contained for packaging")
	flag.Parse()
	if *source == "" {
		fatal("--source is required")
	}
	if *validatePackageSource {
		if err := installconfig.ValidatePackageSource(*source); err != nil {
			fatal(err.Error())
		}
		fmt.Println("package source is self-contained")
		return
	}
	if os.Geteuid() != 0 {
		fatal("opensurge-install-config must run as root")
	}
	if *output == "" {
		*output = filepath.Join(*root, "config.yaml")
	}
	cfg, err := installconfig.Prepare(*source, *root)
	if err != nil {
		fatal(err.Error())
	}
	if err := installconfig.Write(cfg, *output); err != nil {
		fatal(err.Error())
	}
	fmt.Println(*output)
}

func fatal(message string) {
	fmt.Fprintln(os.Stderr, message)
	os.Exit(1)
}
