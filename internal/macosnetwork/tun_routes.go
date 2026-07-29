package macosnetwork

import (
	"context"
	"fmt"
	"strings"
)

type RouteSelection struct {
	Interface string `json:"interface"`
	Gateway   string `json:"gateway,omitempty"`
}

func LookupRoute(ctx context.Context, destination string) (RouteSelection, error) {
	output, err := runCommand(ctx, "/sbin/route", "-n", "get", destination)
	if err != nil {
		return RouteSelection{}, err
	}
	route := parseRouteGet(output)
	if route.Interface == "" {
		return RouteSelection{}, fmt.Errorf("route lookup for %s did not report an interface", destination)
	}
	return RouteSelection{Interface: route.Interface, Gateway: route.Gateway}, nil
}

type routeGetResult struct {
	Interface string
	Gateway   string
}

func parseRouteGet(output string) routeGetResult {
	var result routeGetResult
	for _, line := range strings.Split(output, "\n") {
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		switch strings.TrimSpace(key) {
		case "interface":
			result.Interface = strings.TrimSpace(value)
		case "gateway":
			result.Gateway = strings.TrimSpace(value)
		}
	}
	return result
}
