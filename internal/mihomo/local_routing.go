package mihomo

import (
	"context"
	"fmt"
	"strings"
	"time"

	"open-mihomo-gateway/internal/config"
)

const (
	LocalRoutingModeRule   = "rule"
	LocalRoutingModeGlobal = "global"
	LocalRoutingModeDirect = "direct"

	LocalRoutingGlobalGroup = "open-surge/mac-global"
	LocalRoutingTCPGroup    = "open-surge/mac-mode-tcp"
	LocalRoutingUDPGroup    = "open-surge/mac-mode-udp"
	LocalRoutingGroupPrefix = "open-surge/mac-"

	localRoutingTUNSource = "198.18.0.1/32"
)

type LocalRoutingSnapshot struct {
	Mode               string      `json:"mode"`
	AvailableModes     []string    `json:"available_modes"`
	GlobalGroup        *ProxyGroup `json:"global_group,omitempty"`
	UDPBehavior        string      `json:"udp_behavior"`
	Transports         []string    `json:"transports"`
	NewConnectionsOnly bool        `json:"new_connections_only"`
	Consistent         bool        `json:"consistent"`
	Warning            string      `json:"warning,omitempty"`
}

type localRoutingGeneratedGroup struct {
	Name      string
	Policies  []string
	Providers []string
}

type localRoutingGeneratedPolicy struct {
	Groups []localRoutingGeneratedGroup
	Rules  []string
}

func IsLocalRoutingGroup(name string) bool {
	return strings.HasPrefix(name, LocalRoutingGroupPrefix)
}

func VisibleProxyGroups(groups []ProxyGroup) []ProxyGroup {
	visible := make([]ProxyGroup, 0, len(groups))
	for _, group := range groups {
		if IsLocalRoutingGroup(group.Name) {
			continue
		}
		group.Options = visiblePolicyNames(group.Options)
		if IsLocalRoutingGroup(group.Selected) {
			group.Selected = ""
		}
		visible = append(visible, group)
	}
	return visible
}

func VisibleProxyHealth(proxies []ProxyHealth) []ProxyHealth {
	visible := make([]ProxyHealth, 0, len(proxies))
	for _, proxy := range proxies {
		if IsLocalRoutingGroup(proxy.Name) {
			continue
		}
		if IsLocalRoutingGroup(proxy.Selected) {
			proxy.Selected = ""
		}
		visible = append(visible, proxy)
	}
	return visible
}

func VisibleProviders(snapshot ProvidersSnapshot) ProvidersSnapshot {
	providers := make([]ProxyProvider, 0, len(snapshot.ProxyProviders))
	for _, provider := range snapshot.ProxyProviders {
		if IsLocalRoutingGroup(provider.Name) {
			continue
		}
		visible := make([]ProviderProxy, 0, len(provider.Proxies))
		for _, proxy := range provider.Proxies {
			if !IsLocalRoutingGroup(proxy.Name) {
				visible = append(visible, proxy)
			}
		}
		provider.Proxies = visible
		provider.ProxyCount = len(visible)
		providers = append(providers, provider)
	}
	snapshot.ProxyProviders = providers
	return snapshot
}

func visiblePolicyNames(names []string) []string {
	visible := make([]string, 0, len(names))
	for _, name := range names {
		if !IsLocalRoutingGroup(name) {
			visible = append(visible, name)
		}
	}
	return visible
}

func FetchLocalRouting(ctx context.Context, cfg config.Config) (LocalRoutingSnapshot, error) {
	groups, err := FetchProxyGroups(ctx, cfg)
	if err != nil {
		return LocalRoutingSnapshot{}, err
	}
	return localRoutingSnapshot(cfg, groups), nil
}

func SetLocalRouting(ctx context.Context, cfg config.Config, mode, globalPolicy string) (LocalRoutingSnapshot, error) {
	mode = strings.ToLower(strings.TrimSpace(mode))
	switch mode {
	case LocalRoutingModeRule, LocalRoutingModeGlobal, LocalRoutingModeDirect:
	default:
		return LocalRoutingSnapshot{}, fmt.Errorf("local routing mode must be rule, global, or direct")
	}

	groups, err := FetchProxyGroups(ctx, cfg)
	if err != nil {
		return LocalRoutingSnapshot{}, err
	}
	tcpGroup, tcpOK := proxyGroupByName(groups, LocalRoutingTCPGroup)
	udpGroup, udpOK := proxyGroupByName(groups, LocalRoutingUDPGroup)
	if !tcpOK || !udpOK {
		return LocalRoutingSnapshot{}, fmt.Errorf("local Mac routing groups are not available; reload or restart the gateway with the current OpenSurge configuration")
	}
	globalGroup, globalOK := proxyGroupByName(groups, LocalRoutingGlobalGroup)

	selectedGlobal := strings.TrimSpace(globalPolicy)
	if selectedGlobal == "" && globalOK {
		selectedGlobal = globalGroup.Selected
	}
	if selectedGlobal != "" {
		if !globalOK || !proxyGroupHasOption(globalGroup, selectedGlobal) {
			return LocalRoutingSnapshot{}, fmt.Errorf("global policy %q is not available for local Mac routing", selectedGlobal)
		}
	}
	if mode == LocalRoutingModeGlobal && (!globalOK || selectedGlobal == "") {
		return LocalRoutingSnapshot{}, fmt.Errorf("global mode requires at least one imported or managed proxy policy")
	}

	tcpTarget, udpTarget := "PASS", "PASS"
	switch mode {
	case LocalRoutingModeDirect:
		tcpTarget, udpTarget = "DIRECT", "DIRECT"
	case LocalRoutingModeGlobal:
		tcpTarget = LocalRoutingGlobalGroup
		udpTarget = LocalRoutingGlobalGroup
		health, healthErr := FetchProxyHealth(ctx, cfg)
		if healthErr != nil || !proxySupportsUDP(health.Proxies, selectedGlobal) {
			udpTarget = "REJECT"
		}
	}
	if !proxyGroupHasOption(tcpGroup, tcpTarget) || !proxyGroupHasOption(udpGroup, udpTarget) {
		return LocalRoutingSnapshot{}, fmt.Errorf("generated local Mac routing groups do not support the requested mode")
	}

	type selection struct {
		group string
		from  string
		to    string
	}
	changes := make([]selection, 0, 3)
	if globalOK && selectedGlobal != "" && selectedGlobal != globalGroup.Selected {
		changes = append(changes, selection{group: LocalRoutingGlobalGroup, from: globalGroup.Selected, to: selectedGlobal})
	}
	if tcpTarget != tcpGroup.Selected {
		changes = append(changes, selection{group: LocalRoutingTCPGroup, from: tcpGroup.Selected, to: tcpTarget})
	}
	if udpTarget != udpGroup.Selected {
		changes = append(changes, selection{group: LocalRoutingUDPGroup, from: udpGroup.Selected, to: udpTarget})
	}

	applied := make([]selection, 0, len(changes))
	rollback := func() {
		rollbackCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		for i := len(applied) - 1; i >= 0; i-- {
			_ = SelectProxyGroup(rollbackCtx, cfg, applied[i].group, applied[i].from)
		}
	}
	for _, change := range changes {
		if err := SelectProxyGroup(ctx, cfg, change.group, change.to); err != nil {
			rollback()
			return LocalRoutingSnapshot{}, fmt.Errorf("select %s: %w", change.group, err)
		}
		applied = append(applied, change)
	}

	updated, err := FetchProxyGroups(ctx, cfg)
	if err != nil {
		rollback()
		return LocalRoutingSnapshot{}, err
	}
	snapshot := localRoutingSnapshot(cfg, updated)
	if !snapshot.Consistent || snapshot.Mode != mode {
		rollback()
		return LocalRoutingSnapshot{}, fmt.Errorf("mihomo did not apply local Mac routing mode %q consistently", mode)
	}
	return snapshot, nil
}

func localRoutingSnapshot(cfg config.Config, groups []ProxyGroup) LocalRoutingSnapshot {
	snapshot := LocalRoutingSnapshot{
		Mode:               LocalRoutingModeRule,
		AvailableModes:     []string{LocalRoutingModeRule, LocalRoutingModeDirect},
		UDPBehavior:        "rules",
		Transports:         []string{"loopback_explicit_proxy"},
		NewConnectionsOnly: true,
	}
	if cfg.Transparent.TUNEnabled() {
		snapshot.Transports = append([]string{"tun"}, snapshot.Transports...)
	}

	tcp, tcpOK := proxyGroupByName(groups, LocalRoutingTCPGroup)
	udp, udpOK := proxyGroupByName(groups, LocalRoutingUDPGroup)
	global, globalOK := proxyGroupByName(groups, LocalRoutingGlobalGroup)
	if globalOK {
		copy := global
		snapshot.GlobalGroup = &copy
		snapshot.AvailableModes = append(snapshot.AvailableModes, LocalRoutingModeGlobal)
	}
	if !tcpOK || !udpOK {
		snapshot.Warning = "本机流量模式尚未应用；请使用当前配置重载或重启网关。"
		return snapshot
	}

	switch {
	case tcp.Selected == "PASS" && udp.Selected == "PASS":
		snapshot.Mode = LocalRoutingModeRule
		snapshot.UDPBehavior = "rules"
		snapshot.Consistent = true
	case tcp.Selected == "DIRECT" && udp.Selected == "DIRECT":
		snapshot.Mode = LocalRoutingModeDirect
		snapshot.UDPBehavior = "direct"
		snapshot.Consistent = true
	case globalOK && tcp.Selected == LocalRoutingGlobalGroup && (udp.Selected == LocalRoutingGlobalGroup || udp.Selected == "REJECT"):
		snapshot.Mode = LocalRoutingModeGlobal
		if udp.Selected == "REJECT" {
			snapshot.UDPBehavior = "reject"
			snapshot.Warning = "当前全局出口不支持 UDP；Mac 本机 UDP 将被拒绝，避免静默落回网关规则或直连。"
		} else {
			snapshot.UDPBehavior = "proxy"
		}
		snapshot.Consistent = true
	default:
		snapshot.Warning = "本机 TCP 与 UDP 模式状态不一致；请重新选择规则、全局或直连。"
	}
	return snapshot
}

func buildLocalRoutingPolicy(cfg config.Config, imported *importedProfile) localRoutingGeneratedPolicy {
	globalPolicies := []string{}
	globalProviders := []string{}
	if imported != nil {
		globalPolicies = append(globalPolicies, imported.inventory.proxies...)
		globalPolicies = append(globalPolicies, imported.inventory.proxyGroups...)
		globalProviders = append(globalProviders, imported.inventory.proxyProviderNames...)
	} else if cfg.UpstreamProxy.Enabled {
		globalPolicies = append(globalPolicies, "open-surge-egress")
	}

	groups := []localRoutingGeneratedGroup{}
	tcpPolicies := []string{"PASS", "DIRECT"}
	udpPolicies := []string{"PASS", "DIRECT", "REJECT"}
	if len(globalPolicies) > 0 || len(globalProviders) > 0 {
		groups = append(groups, localRoutingGeneratedGroup{
			Name:      LocalRoutingGlobalGroup,
			Policies:  globalPolicies,
			Providers: globalProviders,
		})
		tcpPolicies = append(tcpPolicies, LocalRoutingGlobalGroup)
		udpPolicies = append(udpPolicies, LocalRoutingGlobalGroup)
	}
	groups = append(groups,
		localRoutingGeneratedGroup{Name: LocalRoutingTCPGroup, Policies: tcpPolicies},
		localRoutingGeneratedGroup{Name: LocalRoutingUDPGroup, Policies: udpPolicies},
	)

	inbounds := [][]string{
		{"IN-TYPE,SOCKS/HTTP", "SRC-IP-CIDR,127.0.0.0/8"},
		{"IN-TYPE,SOCKS/HTTP", "SRC-IP-CIDR," + cfg.Gateway.LANIP + "/32"},
	}
	if cfg.Transparent.TUNEnabled() {
		inbounds = append([][]string{{"IN-TYPE,TUN", "SRC-IP-CIDR," + localRoutingTUNSource}}, inbounds...)
	}
	rules := make([]string, 0, len(inbounds)*(len(dedicatedLocalCIDRs)+2))
	for _, inbound := range inbounds {
		for _, cidr := range dedicatedLocalCIDRs {
			rules = append(rules, localRoutingRule(inbound, "IP-CIDR,"+cidr, "DIRECT"))
		}
		rules = append(rules,
			localRoutingRule(inbound, "NETWORK,TCP", LocalRoutingTCPGroup),
			localRoutingRule(inbound, "NETWORK,UDP", LocalRoutingUDPGroup),
		)
	}
	return localRoutingGeneratedPolicy{Groups: groups, Rules: rules}
}

func localRoutingRule(inbound []string, extra, target string) string {
	parts := append(append([]string{}, inbound...), extra)
	return "AND,((" + strings.Join(parts, "),(") + "))," + target
}

func proxyGroupByName(groups []ProxyGroup, name string) (ProxyGroup, bool) {
	for _, group := range groups {
		if group.Name == name {
			return group, true
		}
	}
	return ProxyGroup{}, false
}

func proxyGroupHasOption(group ProxyGroup, option string) bool {
	for _, candidate := range group.Options {
		if candidate == option {
			return true
		}
	}
	return false
}

func proxySupportsUDP(proxies []ProxyHealth, name string) bool {
	for _, proxy := range proxies {
		if proxy.Name == name {
			return proxy.UDP
		}
	}
	return false
}
