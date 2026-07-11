package device

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// PolicySet is the declarative, gateway-owned source of per-device routing.
// It deliberately contains no proxy credentials or complete mihomo profile:
// those continue to live in the managed or imported profile selected by the
// gateway config.
type PolicySet struct {
	Devices   []ManagedDevice `json:"devices"`
	Profiles  []Profile       `json:"profiles"`
	RuleSets  []RuleSet       `json:"rule_sets"`
	Templates []Template      `json:"templates"`
}

type ManagedDevice struct {
	ID      string `json:"id"`
	MAC     string `json:"mac"`
	IPv4    string `json:"ipv4"`
	Profile string `json:"profile"`
}

// Profile is a reusable routing policy. Every device that uses it still gets
// its own mihomo selector groups after compilation.
type Profile struct {
	ID              string   `json:"id"`
	Template        string   `json:"template,omitempty"`
	DefaultPolicies []string `json:"default_policies,omitempty"`
	// OnUnsupported determines what happens when the selected default policy
	// cannot carry a request (notably UDP). The safe default is reject.
	OnUnsupported string `json:"on_unsupported,omitempty"`
	Rules         []Rule `json:"rules,omitempty"`
}

// Template is an optional profile starting point. It carries no built-in rule
// content; users may provide their own templates without modifying the binary.
type Template struct {
	ID              string   `json:"id"`
	DefaultPolicies []string `json:"default_policies"`
	OnUnsupported   string   `json:"on_unsupported,omitempty"`
	Rules           []Rule   `json:"rules,omitempty"`
}

type Rule struct {
	ID            string    `json:"id"`
	Match         RuleMatch `json:"match"`
	Action        string    `json:"action,omitempty"`
	Policies      []string  `json:"policies,omitempty"`
	OnUnsupported string    `json:"on_unsupported,omitempty"`
}

// RuleMatch is an AND across populated fields and an OR within each field.
// For example, domains ["a.example", "b.example"] and protocols ["tcp"]
// compile to two source-IP-and-domain-and-protocol rules.
type RuleMatch struct {
	Domains   []string `json:"domains,omitempty"`
	IPCIDRs   []string `json:"ip_cidrs,omitempty"`
	Protocols []string `json:"protocols,omitempty"`
	Ports     []string `json:"ports,omitempty"`
	RuleSets  []string `json:"rule_sets,omitempty"`
}

// RuleSet maps to a mihomo rule-provider. Inline sets are suitable for small
// local lists; HTTP sets allow an operator-managed large list. The compiler
// only emits a provider when it is referenced by a device rule.
type RuleSet struct {
	ID       string   `json:"id"`
	Type     string   `json:"type,omitempty"`   // inline or http
	Behavior string   `json:"behavior"`         // domain, ipcidr, or classical
	Format   string   `json:"format,omitempty"` // yaml, text, or mrs for HTTP
	URL      string   `json:"url,omitempty"`
	Interval int      `json:"interval,omitempty"`
	Payload  []string `json:"payload,omitempty"`
}

type Reservation struct {
	ID   string `json:"id"`
	MAC  string `json:"mac"`
	IPv4 string `json:"ipv4"`
}

type SelectorGroup struct {
	Name     string   `json:"name"`
	Policies []string `json:"policies"`
}

type RuleProvider struct {
	Name     string   `json:"name"`
	Type     string   `json:"type"`
	Behavior string   `json:"behavior"`
	Format   string   `json:"format,omitempty"`
	URL      string   `json:"url,omitempty"`
	Interval int      `json:"interval,omitempty"`
	Payload  []string `json:"payload,omitempty"`
}

type CompiledDevice struct {
	ID      string            `json:"id"`
	MAC     string            `json:"mac"`
	IPv4    string            `json:"ipv4"`
	Profile string            `json:"profile"`
	Groups  map[string]string `json:"groups"` // slot (default or rule id) -> mihomo group name
}

type CompiledPolicy struct {
	Reservations    []Reservation    `json:"reservations"`
	SelectorGroups  []SelectorGroup  `json:"selector_groups"`
	RuleProviders   []RuleProvider   `json:"rule_providers"`
	OverrideRules   []string         `json:"override_rules"`
	DefaultRules    []string         `json:"default_rules"`
	Devices         []CompiledDevice `json:"devices"`
	SelectorTargets []string         `json:"selector_targets"`
	ActionTargets   []string         `json:"action_targets"`
}

var policyID = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

func LoadPolicySet(path string) (PolicySet, error) {
	if strings.TrimSpace(path) == "" {
		return PolicySet{}, nil
	}
	file, err := os.Open(path)
	if err != nil {
		return PolicySet{}, err
	}
	defer file.Close()

	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	var set PolicySet
	if err := decoder.Decode(&set); err != nil {
		return PolicySet{}, fmt.Errorf("decode device policy file: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return PolicySet{}, fmt.Errorf("decode device policy file: expected one JSON document")
		}
		return PolicySet{}, fmt.Errorf("decode device policy file: %w", err)
	}
	if err := ValidatePolicySet(set); err != nil {
		return PolicySet{}, err
	}
	return set, nil
}

func ValidatePolicySet(set PolicySet) error {
	profiles := make(map[string]Profile, len(set.Profiles))
	for _, profile := range set.Profiles {
		if !validID(profile.ID) {
			return fmt.Errorf("profile id %q must contain only letters, numbers, underscores, or hyphens", profile.ID)
		}
		if _, exists := profiles[profile.ID]; exists {
			return fmt.Errorf("duplicate profile id %q", profile.ID)
		}
		profiles[profile.ID] = profile
	}
	templates := make(map[string]Template, len(set.Templates))
	for _, template := range set.Templates {
		if !validID(template.ID) {
			return fmt.Errorf("template id %q must contain only letters, numbers, underscores, or hyphens", template.ID)
		}
		if _, exists := templates[template.ID]; exists {
			return fmt.Errorf("duplicate template id %q", template.ID)
		}
		if err := validatePolicyList(template.DefaultPolicies, "template "+template.ID+" default_policies"); err != nil {
			return err
		}
		if err := validateUnsupported(template.OnUnsupported, "template "+template.ID+" on_unsupported"); err != nil {
			return err
		}
		templates[template.ID] = template
	}
	sets := make(map[string]RuleSet, len(set.RuleSets))
	for _, ruleSet := range set.RuleSets {
		if !validID(ruleSet.ID) {
			return fmt.Errorf("rule set id %q must contain only letters, numbers, underscores, or hyphens", ruleSet.ID)
		}
		if _, exists := sets[ruleSet.ID]; exists {
			return fmt.Errorf("duplicate rule set id %q", ruleSet.ID)
		}
		if err := validateRuleSet(ruleSet); err != nil {
			return err
		}
		sets[ruleSet.ID] = ruleSet
	}
	for _, template := range set.Templates {
		if err := validateRules("template "+template.ID, template.Rules, sets); err != nil {
			return err
		}
	}

	for _, profile := range set.Profiles {
		if profile.Template != "" {
			if _, exists := templates[profile.Template]; !exists {
				return fmt.Errorf("profile %q references unknown template %q", profile.ID, profile.Template)
			}
		}
		if len(profile.DefaultPolicies) > 0 {
			if err := validatePolicyList(profile.DefaultPolicies, "profile "+profile.ID+" default_policies"); err != nil {
				return err
			}
		}
		if err := validateUnsupported(profile.OnUnsupported, "profile "+profile.ID+" on_unsupported"); err != nil {
			return err
		}
		resolved, err := resolveProfile(profile, templates)
		if err != nil {
			return err
		}
		if len(resolved.DefaultPolicies) == 0 {
			return fmt.Errorf("profile %q requires default_policies or a template with default_policies", profile.ID)
		}
		if err := validateRules(profile.ID, resolved.Rules, sets); err != nil {
			return err
		}
	}

	seenIDs := map[string]bool{}
	seenMACs := map[string]bool{}
	seenIPs := map[string]bool{}
	for _, managed := range set.Devices {
		if !validID(managed.ID) {
			return fmt.Errorf("device id %q must contain only letters, numbers, underscores, or hyphens", managed.ID)
		}
		if seenIDs[managed.ID] {
			return fmt.Errorf("duplicate device id %q", managed.ID)
		}
		seenIDs[managed.ID] = true
		mac, err := normalizedMAC(managed.MAC)
		if err != nil {
			return fmt.Errorf("device %q mac: %w", managed.ID, err)
		}
		if seenMACs[mac] {
			return fmt.Errorf("duplicate device mac %q", mac)
		}
		seenMACs[mac] = true
		ip := net.ParseIP(managed.IPv4).To4()
		if ip == nil {
			return fmt.Errorf("device %q ipv4 must be a valid IPv4 address", managed.ID)
		}
		canonicalIP := ip.String()
		if seenIPs[canonicalIP] {
			return fmt.Errorf("duplicate device ipv4 %q", canonicalIP)
		}
		seenIPs[canonicalIP] = true
		if _, exists := profiles[managed.Profile]; !exists {
			return fmt.Errorf("device %q references unknown profile %q", managed.ID, managed.Profile)
		}
	}
	return nil
}

func ValidatePolicySetForLAN(set PolicySet, gatewayIP string) error {
	return ValidatePolicySetForLANWithProtected(set, gatewayIP, nil)
}

// ValidatePolicySetForLANWithProtected validates reservations against the LAN
// gateway and declared static addresses that must never be reused. It is
// intentionally separate from live ARP probing, which belongs to start-time
// validation on a real L2 network.
func ValidatePolicySetForLANWithProtected(set PolicySet, gatewayIP string, protected []string) error {
	if err := ValidatePolicySet(set); err != nil {
		return err
	}
	lan := net.ParseIP(gatewayIP).To4()
	if lan == nil {
		return fmt.Errorf("gateway LAN IP must be IPv4 when validating device policies")
	}
	protectedIPs := make(map[string]bool, len(protected))
	for _, value := range protected {
		ip := net.ParseIP(value).To4()
		if ip == nil {
			return fmt.Errorf("protected IPv4 %q must be a valid IPv4 address", value)
		}
		if ip[0] != lan[0] || ip[1] != lan[1] || ip[2] != lan[2] {
			return fmt.Errorf("protected IPv4 %s must remain in gateway LAN %d.%d.%d.0/24", ip.String(), lan[0], lan[1], lan[2])
		}
		protectedIPs[ip.String()] = true
	}
	for _, managed := range set.Devices {
		ip := net.ParseIP(managed.IPv4).To4()
		if ip[0] != lan[0] || ip[1] != lan[1] || ip[2] != lan[2] {
			return fmt.Errorf("device %q ipv4 %s must remain in gateway LAN %d.%d.%d.0/24", managed.ID, ip.String(), lan[0], lan[1], lan[2])
		}
		if ip[3] == 0 || ip[3] == 255 {
			return fmt.Errorf("device %q ipv4 %s must not be the gateway LAN network or broadcast address", managed.ID, ip.String())
		}
		if ip.Equal(lan) {
			return fmt.Errorf("device %q ipv4 must differ from gateway.lan_ip", managed.ID)
		}
		if protectedIPs[ip.String()] {
			return fmt.Errorf("device %q ipv4 %s conflicts with a protected IPv4 address", managed.ID, ip.String())
		}
	}
	return nil
}

func CompilePolicySet(set PolicySet) (CompiledPolicy, error) {
	if err := ValidatePolicySet(set); err != nil {
		return CompiledPolicy{}, err
	}
	profiles := make(map[string]Profile, len(set.Profiles))
	for _, profile := range set.Profiles {
		profiles[profile.ID] = profile
	}
	templates := make(map[string]Template, len(set.Templates))
	for _, template := range set.Templates {
		templates[template.ID] = template
	}
	ruleSets := make(map[string]RuleSet, len(set.RuleSets))
	for _, ruleSet := range set.RuleSets {
		ruleSets[ruleSet.ID] = normalizedRuleSet(ruleSet)
	}

	compiled := CompiledPolicy{}
	usedRuleSets := map[string]bool{}
	for _, managed := range set.Devices {
		profile, err := resolveProfile(profiles[managed.Profile], templates)
		if err != nil {
			return CompiledPolicy{}, err
		}
		mac, _ := normalizedMAC(managed.MAC)
		ip := net.ParseIP(managed.IPv4).To4().String()
		device := CompiledDevice{
			ID:      managed.ID,
			MAC:     mac,
			IPv4:    ip,
			Profile: profile.ID,
			Groups:  map[string]string{},
		}
		compiled.Reservations = append(compiled.Reservations, Reservation{ID: device.ID, MAC: mac, IPv4: ip})

		defaultGroup := DeviceGroupName(device.ID, "default")
		device.Groups["default"] = defaultGroup
		compiled.SelectorGroups = append(compiled.SelectorGroups, SelectorGroup{Name: defaultGroup, Policies: append([]string(nil), profile.DefaultPolicies...)})
		compiled.SelectorTargets = append(compiled.SelectorTargets, profile.DefaultPolicies...)

		for _, rule := range profile.Rules {
			action := rule.Action
			unsupported := resolveUnsupported(rule.OnUnsupported, profile.OnUnsupported)
			if len(rule.Policies) > 0 {
				group := DeviceGroupName(device.ID, rule.ID)
				device.Groups[rule.ID] = group
				compiled.SelectorGroups = append(compiled.SelectorGroups, SelectorGroup{Name: group, Policies: append([]string(nil), rule.Policies...)})
				compiled.SelectorTargets = append(compiled.SelectorTargets, rule.Policies...)
				action = group
			} else {
				compiled.ActionTargets = append(compiled.ActionTargets, action)
			}
			variants, referenced, err := ruleVariants(rule.Match, ruleSets)
			if err != nil {
				return CompiledPolicy{}, fmt.Errorf("device %q rule %q: %w", device.ID, rule.ID, err)
			}
			for _, id := range referenced {
				usedRuleSets[id] = true
			}
			for _, variant := range variants {
				payloads := append([]string{"SRC-IP-CIDR," + ip + "/32"}, variant...)
				compiled.OverrideRules = append(compiled.OverrideRules, composeRule(payloads, action))
				if unsupported == "reject" && actionCanFallThrough(action) {
					compiled.OverrideRules = append(compiled.OverrideRules, composeRule(payloads, "REJECT"))
				}
			}
		}
		compiled.DefaultRules = append(compiled.DefaultRules, "SRC-IP-CIDR,"+ip+"/32,"+defaultGroup)
		if resolveUnsupported("", profile.OnUnsupported) == "reject" {
			compiled.DefaultRules = append(compiled.DefaultRules, "SRC-IP-CIDR,"+ip+"/32,REJECT")
		}
		compiled.Devices = append(compiled.Devices, device)
	}

	for _, id := range sortedKeys(usedRuleSets) {
		ruleSet := ruleSets[id]
		compiled.RuleProviders = append(compiled.RuleProviders, RuleProvider{
			Name:     RuleSetProviderName(ruleSet.ID),
			Type:     ruleSet.Type,
			Behavior: ruleSet.Behavior,
			Format:   ruleSet.Format,
			URL:      ruleSet.URL,
			Interval: ruleSet.Interval,
			Payload:  append([]string(nil), ruleSet.Payload...),
		})
	}
	return compiled, nil
}

func DeviceGroupName(deviceID, slot string) string {
	return "device/" + deviceID + "/" + slot
}

func RuleSetProviderName(id string) string {
	return "open-surge-ruleset-" + id
}

func DeviceGroup(set PolicySet, deviceID, slot string) (string, error) {
	compiled, err := CompilePolicySet(set)
	if err != nil {
		return "", err
	}
	for _, device := range compiled.Devices {
		if device.ID != deviceID {
			continue
		}
		group, exists := device.Groups[slot]
		if !exists {
			return "", fmt.Errorf("device %q has no selectable policy slot %q", deviceID, slot)
		}
		return group, nil
	}
	return "", fmt.Errorf("unknown device %q", deviceID)
}

func resolveProfile(profile Profile, templates map[string]Template) (Profile, error) {
	if profile.Template == "" {
		return Profile{ID: profile.ID, DefaultPolicies: append([]string(nil), profile.DefaultPolicies...), OnUnsupported: profile.OnUnsupported, Rules: append([]Rule(nil), profile.Rules...)}, nil
	}
	template, exists := templates[profile.Template]
	if !exists {
		return Profile{}, fmt.Errorf("profile %q references unknown template %q", profile.ID, profile.Template)
	}
	defaultPolicies := append([]string(nil), template.DefaultPolicies...)
	if len(profile.DefaultPolicies) > 0 {
		defaultPolicies = append([]string(nil), profile.DefaultPolicies...)
	}
	rules := append([]Rule(nil), template.Rules...)
	rules = append(rules, profile.Rules...)
	onUnsupported := template.OnUnsupported
	if profile.OnUnsupported != "" {
		onUnsupported = profile.OnUnsupported
	}
	return Profile{ID: profile.ID, DefaultPolicies: defaultPolicies, OnUnsupported: onUnsupported, Rules: rules}, nil
}

func validateRules(profileID string, rules []Rule, ruleSets map[string]RuleSet) error {
	seen := map[string]bool{}
	for _, rule := range rules {
		if !validID(rule.ID) || rule.ID == "default" {
			return fmt.Errorf("profile %q rule id %q must contain only letters, numbers, underscores, or hyphens and must not be default", profileID, rule.ID)
		}
		if seen[rule.ID] {
			return fmt.Errorf("profile %q has duplicate rule id %q", profileID, rule.ID)
		}
		seen[rule.ID] = true
		if err := validateMatch(rule.Match, ruleSets); err != nil {
			return fmt.Errorf("profile %q rule %q: %w", profileID, rule.ID, err)
		}
		if err := validateUnsupported(rule.OnUnsupported, "profile "+profileID+" rule "+rule.ID+" on_unsupported"); err != nil {
			return err
		}
		if len(rule.Policies) > 0 {
			if rule.Action != "" {
				return fmt.Errorf("profile %q rule %q cannot set action when policies creates a selector group", profileID, rule.ID)
			}
			if err := validatePolicyList(rule.Policies, "profile "+profileID+" rule "+rule.ID+" policies"); err != nil {
				return err
			}
			continue
		}
		if !validPolicy(rule.Action) {
			return fmt.Errorf("profile %q rule %q requires action or policies", profileID, rule.ID)
		}
	}
	return nil
}

func validateMatch(match RuleMatch, ruleSets map[string]RuleSet) error {
	if len(match.Domains)+len(match.IPCIDRs)+len(match.Protocols)+len(match.Ports)+len(match.RuleSets) == 0 {
		return fmt.Errorf("match must include domains, ip_cidrs, protocols, ports, or rule_sets")
	}
	for _, domain := range match.Domains {
		if !validDomain(domain) {
			return fmt.Errorf("domain %q must be a domain suffix without whitespace, commas, scheme, or path", domain)
		}
	}
	for _, cidr := range match.IPCIDRs {
		if _, _, err := net.ParseCIDR(cidr); err != nil {
			return fmt.Errorf("ip_cidr %q is invalid", cidr)
		}
	}
	for _, protocol := range match.Protocols {
		switch strings.ToLower(protocol) {
		case "tcp", "udp":
		default:
			return fmt.Errorf("protocol %q must be tcp or udp", protocol)
		}
	}
	for _, port := range match.Ports {
		if !validPortRange(port) {
			return fmt.Errorf("port %q must be a port number or inclusive range", port)
		}
	}
	for _, id := range match.RuleSets {
		if _, exists := ruleSets[id]; !exists {
			return fmt.Errorf("rule_sets references unknown rule set %q", id)
		}
	}
	if combinationCount(match) > 256 {
		return fmt.Errorf("match expands to more than 256 mihomo rules; split the rule or use a rule set")
	}
	return nil
}

func validateRuleSet(ruleSet RuleSet) error {
	ruleSet = normalizedRuleSet(ruleSet)
	switch ruleSet.Type {
	case "inline":
		if len(ruleSet.Payload) == 0 {
			return fmt.Errorf("rule set %q inline type requires payload", ruleSet.ID)
		}
		if ruleSet.URL != "" {
			return fmt.Errorf("rule set %q inline type must not set url", ruleSet.ID)
		}
	case "http":
		parsed, err := url.Parse(ruleSet.URL)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
			return fmt.Errorf("rule set %q http type requires an http or https url", ruleSet.ID)
		}
		if len(ruleSet.Payload) != 0 {
			return fmt.Errorf("rule set %q http type must not set payload", ruleSet.ID)
		}
	default:
		return fmt.Errorf("rule set %q type must be inline or http", ruleSet.ID)
	}
	switch ruleSet.Behavior {
	case "domain", "ipcidr", "classical":
	default:
		return fmt.Errorf("rule set %q behavior must be domain, ipcidr, or classical", ruleSet.ID)
	}
	if ruleSet.Type == "http" {
		switch ruleSet.Format {
		case "yaml", "text", "mrs":
		default:
			return fmt.Errorf("rule set %q format must be yaml, text, or mrs", ruleSet.ID)
		}
		if ruleSet.Format == "mrs" && ruleSet.Behavior == "classical" {
			return fmt.Errorf("rule set %q mrs format supports domain or ipcidr behavior only", ruleSet.ID)
		}
	}
	if ruleSet.Interval < 0 {
		return fmt.Errorf("rule set %q interval must not be negative", ruleSet.ID)
	}
	return nil
}

func normalizedRuleSet(ruleSet RuleSet) RuleSet {
	ruleSet.Type = strings.ToLower(strings.TrimSpace(ruleSet.Type))
	if ruleSet.Type == "" {
		ruleSet.Type = "inline"
	}
	ruleSet.Behavior = strings.ToLower(strings.TrimSpace(ruleSet.Behavior))
	ruleSet.Format = strings.ToLower(strings.TrimSpace(ruleSet.Format))
	if ruleSet.Format == "" {
		ruleSet.Format = "yaml"
	}
	return ruleSet
}

func ruleVariants(match RuleMatch, ruleSets map[string]RuleSet) ([][]string, []string, error) {
	variants := [][]string{{}}
	variants = appendConditionDimension(variants, domainConditions(match.Domains))
	variants = appendConditionDimension(variants, ipCIDRConditions(match.IPCIDRs))
	variants = appendConditionDimension(variants, protocolConditions(match.Protocols))
	variants = appendConditionDimension(variants, portConditions(match.Ports))
	var refs []string
	var ruleSetConditions []string
	for _, id := range match.RuleSets {
		ruleSetConditions = append(ruleSetConditions, "RULE-SET,"+RuleSetProviderName(id))
		refs = append(refs, id)
	}
	variants = appendConditionDimension(variants, ruleSetConditions)
	if len(variants) == 0 || len(variants[0]) == 0 {
		return nil, nil, fmt.Errorf("match must not be empty")
	}
	return variants, refs, nil
}

func appendConditionDimension(existing [][]string, conditions []string) [][]string {
	if len(conditions) == 0 {
		return existing
	}
	next := make([][]string, 0, len(existing)*len(conditions))
	for _, prior := range existing {
		for _, condition := range conditions {
			variant := append([]string(nil), prior...)
			variant = append(variant, condition)
			next = append(next, variant)
		}
	}
	return next
}

func domainConditions(domains []string) []string {
	conditions := make([]string, 0, len(domains))
	for _, domain := range domains {
		conditions = append(conditions, "DOMAIN-SUFFIX,"+strings.ToLower(domain))
	}
	return conditions
}

func ipCIDRConditions(cidrs []string) []string {
	conditions := make([]string, 0, len(cidrs))
	for _, cidr := range cidrs {
		conditions = append(conditions, "IP-CIDR,"+cidr)
	}
	return conditions
}

func protocolConditions(protocols []string) []string {
	conditions := make([]string, 0, len(protocols))
	for _, protocol := range protocols {
		conditions = append(conditions, "NETWORK,"+strings.ToLower(protocol))
	}
	return conditions
}

func portConditions(ports []string) []string {
	conditions := make([]string, 0, len(ports))
	for _, port := range ports {
		conditions = append(conditions, "DST-PORT,"+port)
	}
	return conditions
}

func composeRule(payloads []string, action string) string {
	if len(payloads) == 1 {
		return payloads[0] + "," + action
	}
	wrapped := make([]string, 0, len(payloads))
	for _, payload := range payloads {
		wrapped = append(wrapped, "("+payload+")")
	}
	return "AND,(" + strings.Join(wrapped, ",") + ")," + action
}

func combinationCount(match RuleMatch) int {
	count := 1
	for _, dimension := range [][]string{match.Domains, match.IPCIDRs, match.Protocols, match.Ports, match.RuleSets} {
		if len(dimension) > 0 {
			count *= len(dimension)
		}
	}
	return count
}

func normalizedMAC(value string) (string, error) {
	mac, err := net.ParseMAC(value)
	if err != nil || len(mac) != 6 {
		return "", fmt.Errorf("must be an IEEE 802 6-byte MAC address")
	}
	return strings.ToLower(mac.String()), nil
}

func validID(value string) bool {
	return policyID.MatchString(value)
}

func validPolicy(value string) bool {
	return value != "" && strings.TrimSpace(value) == value && !strings.ContainsAny(value, ",\t\r\n")
}

func validateUnsupported(value string, label string) error {
	switch value {
	case "", "reject", "fallthrough":
		return nil
	default:
		return fmt.Errorf("%s must be reject or fallthrough", label)
	}
}

func resolveUnsupported(ruleValue, profileValue string) string {
	if ruleValue != "" {
		return ruleValue
	}
	if profileValue != "" {
		return profileValue
	}
	return "reject"
}

func actionCanFallThrough(action string) bool {
	switch strings.ToUpper(action) {
	case "DIRECT", "REJECT", "REJECT-DROP", "REJECT-TINYGIF":
		return false
	default:
		return true
	}
}

func validatePolicyList(policies []string, label string) error {
	if len(policies) == 0 {
		return fmt.Errorf("%s must not be empty", label)
	}
	seen := map[string]bool{}
	for _, policy := range policies {
		if !validPolicy(policy) {
			return fmt.Errorf("%s contains invalid policy %q", label, policy)
		}
		if seen[policy] {
			return fmt.Errorf("%s contains duplicate policy %q", label, policy)
		}
		seen[policy] = true
	}
	return nil
}

func validDomain(value string) bool {
	return value != "" && strings.TrimSpace(value) == value && !strings.ContainsAny(value, ",\t\r\n/: ")
}

func validPortRange(value string) bool {
	parts := strings.Split(value, "-")
	if len(parts) > 2 || value == "" {
		return false
	}
	first, err := strconv.Atoi(parts[0])
	if err != nil || first < 1 || first > 65535 {
		return false
	}
	if len(parts) == 1 {
		return true
	}
	last, err := strconv.Atoi(parts[1])
	return err == nil && last >= first && last <= 65535
}

func sortedKeys(values map[string]bool) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
