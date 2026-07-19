package macosnetwork

import "testing"

func TestParseNeighborsKeepsCompleteIPv4EntriesOnRequestedInterface(t *testing.T) {
	output := `? (192.168.5.124) at AA:BB:CC:DD:EE:24 on en0 ifscope [ethernet]
? (192.168.5.125) at (incomplete) on en0 ifscope [ethernet]
? (192.168.5.126) at aa:bb:cc:dd:ee:26 on en7 ifscope [ethernet]
? (224.0.0.251) at 1:0:5e:0:0:fb on en0 ifscope permanent [ethernet]
`
	neighbors := parseNeighbors(output, "en0")
	if len(neighbors) != 1 {
		t.Fatalf("parseNeighbors() = %#v", neighbors)
	}
	if neighbors[0].IP != "192.168.5.124" || neighbors[0].MAC != "aa:bb:cc:dd:ee:24" || neighbors[0].Interface != "en0" {
		t.Fatalf("first neighbor = %#v", neighbors[0])
	}
}
