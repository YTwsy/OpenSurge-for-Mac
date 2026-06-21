package doctor

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"

	"open-mihomo-gateway/internal/config"
)

type Check struct {
	Name    string
	OK      bool
	Message string
}

type Report struct {
	Checks []Check
}

func Run(cfg config.Config) Report {
	checks := []Check{
		checkRoot(),
		checkCommand("dnsmasq", "dnsmasq"),
		checkPath("mihomo", cfg.Mihomo.Binary),
		checkCommand("pfctl", "pfctl"),
		checkInterface(cfg.Gateway.Interface),
		checkIPv4("LAN IP", cfg.Gateway.LANIP),
	}
	return Report{Checks: checks}
}

func (r Report) Healthy() bool {
	for _, check := range r.Checks {
		if !check.OK {
			return false
		}
	}
	return true
}

func (r Report) Format() string {
	var out strings.Builder
	for _, check := range r.Checks {
		status := "FAIL"
		if check.OK {
			status = "OK"
		}
		fmt.Fprintf(&out, "[%s] %s", status, check.Name)
		if check.Message != "" {
			fmt.Fprintf(&out, " - %s", check.Message)
		}
		out.WriteByte('\n')
	}
	if r.Healthy() {
		out.WriteString("Doctor: healthy\n")
	} else {
		out.WriteString("Doctor: issues found\n")
	}
	return out.String()
}

func checkRoot() Check {
	if os.Geteuid() == 0 {
		return Check{Name: "root privileges", OK: true}
	}
	return Check{Name: "root privileges", OK: false, Message: "start/stop require sudo"}
}

func checkCommand(name, command string) Check {
	path, err := exec.LookPath(command)
	if err != nil {
		return Check{Name: name, OK: false, Message: "not found in PATH"}
	}
	return Check{Name: name, OK: true, Message: path}
}

func checkPath(name, path string) Check {
	if strings.ContainsRune(path, os.PathSeparator) {
		info, err := os.Stat(path)
		if err != nil {
			return Check{Name: name, OK: false, Message: err.Error()}
		}
		if info.IsDir() {
			return Check{Name: name, OK: false, Message: "path is a directory"}
		}
		return Check{Name: name, OK: true, Message: path}
	}
	return checkCommand(name, path)
}

func checkInterface(name string) Check {
	iface, err := net.InterfaceByName(name)
	if err != nil {
		return Check{Name: "interface " + name, OK: false, Message: err.Error()}
	}
	return Check{Name: "interface " + name, OK: true, Message: iface.HardwareAddr.String()}
}

func checkIPv4(name, value string) Check {
	if net.ParseIP(value).To4() == nil {
		return Check{Name: name, OK: false, Message: "invalid IPv4 address"}
	}
	return Check{Name: name, OK: true, Message: value}
}
