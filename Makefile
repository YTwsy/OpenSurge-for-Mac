.PHONY: test build doctor status policy-control-test
.PHONY: web-install web-build web-test control-build control-run menubar-build menubar-test gui-build gui-test gui-installer gui-notarize
.PHONY: lab-install lab-uninstall-root lab-check lab-up lab-status lab-test
.PHONY: lab-test-tun lab-test-tun-imported-profile lab-test-tun-imported-egress lab-test-tun-device-policy lab-down lab-destroy
.PHONY: real-device-start-off real-device-start-tun real-device-start-tun-proxy
.PHONY: real-device-stop real-device-status real-device-client-check
.PHONY: same-lan-start-tun same-lan-start-tun-proxy same-lan-start-tun-imported-egress
.PHONY: same-lan-stop same-lan-status same-lan-adb-check same-lan-adb-check-imported-egress
.PHONY: same-lan-start-wifi-dhcp-imported-egress same-lan-adb-check-wifi-dhcp-imported-egress
.PHONY: same-lan-stop-wifi-dhcp same-lan-status-wifi-dhcp
.PHONY: same-wifi-dhcp-start-imported-egress same-wifi-dhcp-adb-check-imported-egress
.PHONY: same-wifi-dhcp-start-device-policy same-wifi-dhcp-adb-check-device-policy same-wifi-dhcp-verify-device-policy-recovery
.PHONY: same-wifi-dhcp-stop same-wifi-dhcp-status

test:
	go test ./...

build:
	go build -o bin/omg ./cmd/omg

web-install:
	cd web && pnpm install

web-build:
	cd web && pnpm run build

web-test:
	cd web && pnpm run test

control-build: web-build
	go build -o bin/opensurge-control ./cmd/opensurge-control
	go build -o bin/opensurge-helper ./cmd/opensurge-helper
	go build -o bin/opensurge-install-config ./cmd/opensurge-install-config

control-run: control-build
	./bin/opensurge-control --config examples/config.example.yaml

menubar-build:
	./scripts/build-menubar-app.sh

menubar-test:
	./scripts/check-menubar.sh

gui-build: control-build menubar-build

gui-test: test web-test menubar-test

gui-installer:
	./scripts/build-gui-installer.sh

gui-notarize:
	@test -n "$(PKG)" || (echo "usage: make gui-notarize PKG=/path/to/OpenSurge.pkg" >&2; exit 1)
	./scripts/notarize-gui-installer.sh "$(PKG)"

doctor:
	go run ./cmd/omg doctor --config examples/config.example.yaml

status:
	go run ./cmd/omg status --config examples/config.example.yaml

policy-control-test:
	./tests/integration/policy-control.sh

lab-install:
	./tests/lab/install-host-deps.sh

lab-uninstall-root:
	./tests/lab/install-host-deps.sh --uninstall-root

lab-check:
	./tests/lab/lab.sh check

lab-up:
	./tests/lab/lab.sh up

lab-status:
	./tests/lab/lab.sh status

lab-test:
	./tests/lab/lab.sh test

lab-test-tun:
	./tests/lab/lab.sh test-tun

lab-test-tun-imported-profile:
	OMG_LAB_MIHOMO_PROFILE=tests/lab/mihomo-profile.imported-tun.yaml ./tests/lab/lab.sh test-tun

lab-test-tun-imported-egress:
	OMG_LAB_MIHOMO_PROFILE=tests/lab/mihomo-profile.imported-tun-egress.yaml ./tests/lab/lab.sh test-tun

lab-test-tun-device-policy:
	./tests/lab/lab.sh test-tun-device-policy

lab-down:
	./tests/lab/lab.sh down

lab-destroy:
	./tests/lab/lab.sh destroy

real-device-start-off:
	./tests/real-device/smoke.sh start-off

real-device-start-tun:
	./tests/real-device/smoke.sh start-tun

real-device-start-tun-proxy:
	OMG_REAL_DEVICE_UPSTREAM_PROXY_ENABLED=true ./tests/real-device/smoke.sh start-tun

real-device-stop:
	./tests/real-device/smoke.sh stop

real-device-status:
	./tests/real-device/smoke.sh status

real-device-client-check:
	./tests/real-device/smoke.sh client-check

same-lan-start-tun:
	./tests/same-lan/smoke.sh start-tun

same-lan-start-tun-proxy:
	OMG_SAME_LAN_UPSTREAM_PROXY_ENABLED=true ./tests/same-lan/smoke.sh start-tun

same-lan-start-tun-imported-egress:
	OMG_SAME_LAN_IMPORTED_EGRESS=true ./tests/same-lan/smoke.sh start-tun-imported-egress

same-lan-stop:
	./tests/same-lan/smoke.sh stop

same-lan-status:
	./tests/same-lan/smoke.sh status

same-lan-adb-check:
	./tests/same-lan/smoke.sh adb-check

same-lan-adb-check-imported-egress:
	OMG_SAME_LAN_IMPORTED_EGRESS=true ./tests/same-lan/smoke.sh adb-check-imported-egress

same-lan-start-wifi-dhcp-imported-egress:
	OMG_SAME_WIFI_DHCP_ENABLED=true OMG_SAME_LAN_IMPORTED_EGRESS=true ./tests/same-lan/smoke.sh start-wifi-dhcp-imported-egress

same-lan-adb-check-wifi-dhcp-imported-egress:
	OMG_SAME_WIFI_DHCP_ENABLED=true OMG_SAME_LAN_IMPORTED_EGRESS=true ./tests/same-lan/smoke.sh adb-check-wifi-dhcp-imported-egress

same-lan-stop-wifi-dhcp:
	OMG_SAME_WIFI_DHCP_ENABLED=true OMG_SAME_LAN_IMPORTED_EGRESS=true ./tests/same-lan/smoke.sh stop

same-lan-status-wifi-dhcp:
	OMG_SAME_WIFI_DHCP_ENABLED=true ./tests/same-lan/smoke.sh status

same-wifi-dhcp-start-imported-egress: same-lan-start-wifi-dhcp-imported-egress

same-wifi-dhcp-adb-check-imported-egress: same-lan-adb-check-wifi-dhcp-imported-egress

same-wifi-dhcp-start-device-policy:
	OMG_SAME_WIFI_DHCP_ENABLED=true OMG_SAME_LAN_IMPORTED_EGRESS=true OMG_SAME_WIFI_DEVICE_POLICY_ENABLED=true ./tests/same-lan/smoke.sh start-wifi-dhcp-device-policy

same-wifi-dhcp-adb-check-device-policy:
	OMG_SAME_WIFI_DHCP_ENABLED=true OMG_SAME_LAN_IMPORTED_EGRESS=true OMG_SAME_WIFI_DEVICE_POLICY_ENABLED=true ./tests/same-lan/smoke.sh adb-check-wifi-dhcp-device-policy

same-wifi-dhcp-verify-device-policy-recovery:
	OMG_SAME_WIFI_DHCP_ENABLED=true OMG_SAME_WIFI_DEVICE_POLICY_ENABLED=true ./tests/same-lan/smoke.sh verify-wifi-dhcp-device-policy-recovery

same-wifi-dhcp-stop: same-lan-stop-wifi-dhcp

same-wifi-dhcp-status: same-lan-status-wifi-dhcp
