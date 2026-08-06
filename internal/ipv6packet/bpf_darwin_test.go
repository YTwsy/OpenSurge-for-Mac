//go:build darwin

package ipv6packet

import (
	"syscall"
	"testing"
	"unsafe"
)

func TestParseBPFFramesAcceptsUnpaddedKernelHeaderLength(t *testing.T) {
	frame := []byte{0, 1, 2, 3, 4, 5}
	buffer := make([]byte, bpfWordAlign(bpfHeaderMinimum+len(frame)))
	header := (*syscall.BpfHdr)(unsafe.Pointer(&buffer[0]))
	header.Hdrlen = uint16(bpfHeaderMinimum)
	header.Caplen = uint32(len(frame))
	header.Datalen = uint32(len(frame))
	copy(buffer[bpfHeaderMinimum:], frame)

	frames := parseBPFFrames(buffer)
	if len(frames) != 1 || string(frames[0]) != string(frame) {
		t.Fatalf("frames = %#v", frames)
	}
}
