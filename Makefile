.PHONY: test build doctor status lab-install lab-uninstall-root lab-check lab-up lab-status lab-test lab-test-tun lab-down lab-destroy real-device-start-off real-device-start-tun real-device-stop real-device-status real-device-client-check

test:
	go test ./...

build:
	go build -o bin/omg ./cmd/omg

doctor:
	go run ./cmd/omg doctor --config examples/config.example.yaml

status:
	go run ./cmd/omg status --config examples/config.example.yaml

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

lab-down:
	./tests/lab/lab.sh down

lab-destroy:
	./tests/lab/lab.sh destroy

real-device-start-off:
	./tests/real-device/smoke.sh start-off

real-device-start-tun:
	./tests/real-device/smoke.sh start-tun

real-device-stop:
	./tests/real-device/smoke.sh stop

real-device-status:
	./tests/real-device/smoke.sh status

real-device-client-check:
	./tests/real-device/smoke.sh client-check
