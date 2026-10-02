#!/usr/bin/env python3
"""Isolated UI fixture, never a gateway. Usage: python3 scripts/desktop-smoke-service.py DIR.

Launch the preview executable with --control-dir DIR. Edit DIR/scenario.json to
expire sessions (generation), simulate offline, or change gateway/recovery state.
Restart this process with the same DIR to exercise discovery on a different port.
DIR/observations.json records requests, mutation bodies and SSE activity without
credentials. This fixture never invokes the Helper, launchctl or network tools.
"""
import argparse
import datetime
import http.cookies
import http.server
import json
import mimetypes
import pathlib
import threading
import time
from email import policy
from email.parser import BytesParser

parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument("directory", type=pathlib.Path)
parser.add_argument("--web-root", type=pathlib.Path, help="serve the built React UI for browser-launch acceptance")
args = parser.parse_args()
args.directory.mkdir(parents=True, exist_ok=True, mode=0o700)
scenario_file = args.directory / "scenario.json"
if not scenario_file.exists():
    scenario_file.write_text(json.dumps({"generation": 1, "offline": False, "gateway": "stopped", "recovery": False}))
lock = threading.Lock()
observations = {"requests": {}, "writes": [], "events_opened": 0, "events_closed": 0}
language, sleep_enabled = "zh-Hans", False
login_state = None
sources = []


def scenario():
    try:
        return json.loads(scenario_file.read_text())
    except (ValueError, OSError):
        return {"generation": 1, "offline": False, "gateway": "stopped", "recovery": False}


def observed(kind, value=None):
    with lock:
        if kind == "requests":
            observations[kind][value] = observations[kind].get(value, 0) + 1
        elif kind == "writes":
            observations[kind].append(value)
        else:
            observations[kind] += 1
        (args.directory / "observations.json").write_text(json.dumps(observations, indent=2))


class Handler(http.server.BaseHTTPRequestHandler):
    protocol_version = "HTTP/1.1"

    def log_message(self, *_):
        pass  # Bootstrap grants must not enter URL logs.

    def reply(self, value, status=200, content_type="application/json"):
        data = value if isinstance(value, bytes) else json.dumps(value).encode() if content_type == "application/json" else value.encode()
        self.send_response(status)
        self.send_header("Content-Type", content_type)
        self.send_header("Content-Length", str(len(data)))
        self.end_headers()
        self.wfile.write(data)

    def do_GET(self):
        self.handle_request()

    def do_POST(self):
        self.handle_request()

    def do_PUT(self):
        self.handle_request()

    def read_body(self):
        if self.headers.get("Transfer-Encoding", "").lower() != "chunked":
            return self.rfile.read(int(self.headers.get("Content-Length", 0)))
        chunks = []
        while True:
            size = int(self.rfile.readline().split(b";", 1)[0], 16)
            if not size:
                while self.rfile.readline() != b"\r\n":
                    pass
                return b"".join(chunks)
            chunks.append(self.rfile.read(size))
            self.rfile.read(2)

    def handle_request(self):
        global language, sleep_enabled, login_state
        path = self.path.split("?", 1)[0]
        state = scenario()
        observed("requests", self.command + " " + path)
        if state.get("offline"):
            self.reply({"error": {"code": "desktop_service_unavailable"}}, 503)
            return
        if path == "/api/v1/session/bootstrap":
            self.read_body()
            if self.headers.get("Authorization") != "Bearer smoke-only":
                self.reply({}, 401)
                return
            self.reply({"schema_version": 1, "url": base + "/bootstrap?code=smoke-only", "expires_at": (datetime.datetime.now(datetime.timezone.utc) + datetime.timedelta(seconds=30)).isoformat()}, 201)
            return
        if path == "/bootstrap":
            self.send_response(303)
            self.send_header("Set-Cookie", f"opensurge_session={state['generation']}; HttpOnly; Path=/; Expires=Wed, 01 Jan 2031 00:00:00 GMT")
            self.send_header("Location", "/dashboard")
            self.send_header("Content-Length", "0")
            self.end_headers()
            return
        cookie = http.cookies.SimpleCookie(self.headers.get("Cookie", ""))
        if "opensurge_session" not in cookie or cookie["opensurge_session"].value != str(state["generation"]):
            self.reply({}, 401)
            return
        if args.web_root and self.command == "GET" and not path.startswith("/api/"):
            root = args.web_root.resolve()
            pages = {"/", "/dashboard", "/network", "/sources", "/devices", "/connections", "/policies", "/connectivity", "/diagnostics"}
            asset = (root / ("index.html" if path in pages else path.lstrip("/"))).resolve()
            if not asset.is_relative_to(root) or not asset.is_file():
                self.reply({}, 404)
                return
            self.reply(asset.read_bytes(), content_type=mimetypes.guess_type(asset.name)[0] or "application/octet-stream")
            return
        if path == "/api/v1/events":
            observed("events_opened")
            self.send_response(200)
            self.send_header("Content-Type", "text/event-stream")
            self.end_headers()
            try:
                previous = None
                while not scenario().get("offline"):
                    current = scenario()
                    self.wfile.write(b"event: state\ndata: {}\n\n" if current != previous else b": heartbeat\n\n")
                    self.wfile.flush()
                    previous = current
                    time.sleep(1)
            except (BrokenPipeError, ConnectionResetError):
                pass
            finally:
                observed("events_closed")
                self.close_connection = True
            return
        body = self.read_body()
        payload = {}
        if body:
            if self.headers.get("Content-Type", "").startswith("multipart/form-data"):
                message = BytesParser(policy=policy.default).parsebytes(("Content-Type: " + self.headers["Content-Type"] + "\r\n\r\n").encode() + body)
                files = [{"filename": part.get_filename(), "size": len(part.get_payload(decode=True))} for part in message.iter_parts() if part.get_filename()]
                payload = {"files": files}
            else:
                payload = json.loads(body)
        if self.command != "GET":
            observed("writes", {"method": self.command, "path": path, "body": payload})
        if path == "/api/v1/ui-preferences" and self.command == "PUT":
            language = payload["language"]
        if path == "/api/v1/sleep-prevention" and self.command == "PUT":
            sleep_enabled = payload["enabled"]
        if path == "/api/v1/desktop-smoke/login" and self.command == "PUT":
            if state.get("login_failure"):
                self.reply({}, 503)
                return
            login_state = ("approval" if state.get("login_approval") else "enabled") if payload["enabled"] else "disabled"
        if path == "/api/v1/desktop-smoke/release" and state.get("update_failure"):
            self.reply({}, 503)
            return
        preferences = {"schema_version": 1, "language": language}
        sleep = {"enabled": sleep_enabled, "active": sleep_enabled}
        recovery = {"stage": state.get("recovery_stage", "prepared" if state.get("recovery") else "idle"), "required": state.get("recovery", False), "topology": state.get("mode", "same_lan")}
        snapshot = {"network_service": "Smoke Wi-Fi", "interface": "en0", "ipv4": "192.0.2.10", "subnet_mask": "255.255.255.0", "router": "192.0.2.1", "dns": ["192.0.2.1"], "ipv6_default": False}
        if state.get("recovery"):
            recovery["network_snapshot"] = snapshot
        status = {"gateway": state["gateway"], "interface": "en0", "lan_ip": "192.0.2.10", "dhcp": "stopped", "dhcp_enabled": False, "mihomo": "stopped", "tun": "stopped", "pf_anchor": "unloaded", "forwarding": "disabled", "ipv4_takeover": "stopped", "ipv6_takeover": "disabled", "dns_ipv6": False, "tun_ipv6_requested": "off", "ipv6_packet": "disabled", "native_ipv6_available": False, "client_count": 0}
        common = {"schema_version": 1, "revision": "smoke", "topology": "same_lan", "drift": False, "warnings": [], "doctor_healthy": True, "sleep_prevention": sleep, "ui_preferences": preferences}
        common.update({key: state[key] for key in ("drift", "doctor_healthy", "presentation") if key in state})
        common["topology"] = state.get("mode", "same_lan")
        status.update({key: state[key] for key in ("dhcp", "mihomo", "pf_anchor", "tun", "forwarding", "ipv4_takeover", "ipv6_takeover", "runtime_state") if key in state})
        routes = {
            "/api/v1/desktop-smoke/lifecycle": {},
            "/api/v1/desktop-smoke/login": {"state": login_state or state.get("login_initial", "disabled")},
            "/api/v1/desktop-smoke/uninstall": {"outcome": state.get("uninstall_outcome", "cancel")},
            "/api/v1/desktop-smoke/release": {"tag_name": state.get("release_tag", "v0.2.5"), "html_url": "https://github.com/YTwsy/OpenSurge-for-Mac/releases/tag/" + state.get("release_tag", "v0.2.5"), "draft": False, "prerelease": False},
            "/api/v1/overview": {**common, "status": status, "doctor": [], "leases": [], "policies": [], "providers": {"proxy_providers": [], "rule_providers": []}, "recovery": recovery},
            "/api/v1/menubar": {**common, **status, "recovery_required": recovery["required"], "recovery_stage": recovery["stage"]},
            "/api/v1/ui-preferences": preferences,
            "/api/v1/sleep-prevention": sleep,
            "/api/v1/operations": {"operations": []},
            "/api/v1/config": {"schema_version": 1, "revision": "smoke", "gateway": {"mode": "same_lan", "interface": "en0", "lan_ip": "192.0.2.10", "lan_prefix_len": 24, "upstream_interface": "en0"}, "dhcp": {"enabled": False, "range_start": "192.0.2.100", "range_end": "192.0.2.200", "lease_time": "12h", "domain": "lan", "bypass_gateway": "", "bypass_dns": []}, "dns": {"listen": "192.0.2.10", "upstream": "1.1.1.1", "ipv6": False}, "mihomo": {"store_fake_ip": True}, "transparent": {"mode": "tun", "strict_route": False, "tun_ipv6": "off"}, "local_system_proxy": {"enabled": False}, "device_policy": {"enabled": False, "protected_ipv4": []}},
            "/api/v1/network/interfaces": {"schema_version": 1, "interfaces": [{"interface": "en0", "network_service": "Smoke Wi-Fi"}]},
            "/api/v1/devices": {"drift": False, "applied": False, "devices": [], "leases": [], "observed_devices": []},
            "/api/v1/device-traffic": {"schema_version": 1, "sampled_at": datetime.datetime.now(datetime.timezone.utc).isoformat(), "scope": "active_sessions", "gateway_local": {"ip": "192.0.2.10", "mac": "", "online": False, "active_connections": 0, "upload": 0, "download": 0, "upload_rate": 0, "download_rate": 0}, "devices": [], "totals": {"devices": 0, "active_connections": 0, "upload": 0, "download": 0}, "gateway_rates": {"upload": 0, "download": 0}},
            "/api/v1/local-routing": {"schema_version": 1, "mode": "rule", "available_modes": ["rule", "direct"], "udp_behavior": "rules", "transports": ["tun"], "new_connections_only": True, "consistent": True},
            "/api/v1/diagnostics": {"schema_version": 1, "revision": "smoke", "connections": {"upload_total": 0, "download_total": 0, "connections": []}, "logs": state.get("logs", {}), "operations": [], "recovery": recovery},
            "/api/v1/doctor": {"schema_version": 1, "state": "idle", "current": True, "checks": [], "healthy": True},
            "/api/v1/profile-overlay": {"schema_version": 1, "revision": "smoke", "yaml": "schema-version: 1\nenabled: false\n", "document": {"schema_version": 1, "enabled": False, "rules": {"prepend": [], "append_before_match": []}, "proxies": {"add": [], "replace": []}, "proxy_providers": {"add": {}, "replace": {}}, "proxy_groups": {"add": [], "replace": [], "patch": []}, "rule_providers": {"add": {}, "replace": {}}, "dns": {"merge": {}, "append": {}}}, "desired": True, "applied": False, "validation": "smoke"},
            "/api/v1/tailscale": {"schema_version": 1, "revision": "smoke", "settings": {"enabled": False, "display_name": "Tailnet", "hostname": "smoke", "control_url": "https://controlplane.tailscale.com", "accept_routes": False, "magic_dns_suffixes": [], "peer_cidrs": [], "subnet_routes": [], "allow_mac": False, "allow_all_devices": False, "allowed_devices": [], "exit_node": "", "exit_node_allow_lan_access": False}, "auth_key_present": False, "identity_present": False, "gateway_active": False, "runtime_state": "disabled", "selectable_exit": False, "warnings": []},
        }
        if "traffic" in state:
            routes["/api/v1/device-traffic"].update(state["traffic"])
        traffic = routes["/api/v1/device-traffic"]
        routes["/api/v1/connections"] = {
            **traffic,
            "gateway_totals": {
                **traffic["totals"],
                "active_connections": traffic["gateway_local"]["active_connections"] + traffic["totals"]["active_connections"] + traffic.get("unclassified_connections", 0),
                "upload_rate": traffic["gateway_rates"]["upload"],
                "download_rate": traffic["gateway_rates"]["download"],
            },
            "unclassified": {"key": "unclassified", "identity_source": "unclassified", "ip": "", "mac": "", "online": False, "active_connections": 0, "upload": 0, "download": 0, "upload_rate": 0, "download_rate": 0},
            "connections": state.get("connections", []),
        }
        if "routing" in state:
            routes["/api/v1/local-routing"].update(state["routing"])
        if state.get("traffic_unavailable") and path == "/api/v1/device-traffic":
            self.reply({}, 503)
            return
        routes["/api/v1/config"]["gateway"]["mode"] = state.get("mode", "same_lan")
        routes["/api/v1/gateway/plan"] = {"schema_version": 1, "revision": "smoke", "topology": state.get("mode", "same_lan"), "snapshot": snapshot, "protected_ipv4": ["192.0.2.1", "192.0.2.10"], "dhcp_servers": [], "warnings": [], "blockers": []}
        if path == "/api/v1/sources":
            if self.command == "POST":
                source = {"id": "smoke", "name": payload.get("name", payload.get("files", [{}])[0].get("filename", "smoke")), "kind": "mihomo_profile", "format": "yaml", "enabled": False, "validation": "valid", "applied": False}
                sources.append(source)
                self.reply(source)
            else:
                self.reply({"revision": "smoke", "sources": sources})
        elif path == "/api/v1/recovery/card":
            self.reply("OpenSurge desktop smoke recovery card\nNo real network state.\n", content_type="text/plain; charset=utf-8")
        elif path in routes:
            self.reply(routes[path])
        else:
            self.reply({"error": {"code": "fixture_route_missing", "message": "Smoke fixture has no route: " + path}}, 404)


server = http.server.ThreadingHTTPServer(("127.0.0.1", 0), Handler)
server.daemon_threads = True
base = f"http://127.0.0.1:{server.server_port}"
(args.directory / "control-endpoint.json").write_text(json.dumps({"schema_version": 1, "url": base}))
(args.directory / "control-token").write_text("smoke-only")
(args.directory / "control-token").chmod(0o600)
print(f"Desktop smoke fixture: {base}; discovery: {args.directory}", flush=True)
server.serve_forever()
