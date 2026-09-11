package dhcp

import (
	"reflect"
	"testing"
)

func TestDNSMasqArgsUseProductionForegroundMode(t *testing.T) {
	got := dnsmasqArgs("/tmp/dnsmasq.conf")
	want := []string{"--keep-in-foreground", "--log-facility=-", "--conf-file=/tmp/dnsmasq.conf"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("dnsmasqArgs() = %#v, want %#v", got, want)
	}
}
