package config

// The local management alias is resolved by the live LAN-access lease, not a
// gateway snapshot: its selected management address can differ from LANIP.
const LANShortcutHost = "open.surge"
const LANShortcutDNS = "127.0.0.1:61769"
