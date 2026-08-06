//go:build !with_gvisor

package opensurge_packet

import (
	"fmt"

	"github.com/metacubex/mihomo/adapter/inbound"
	C "github.com/metacubex/mihomo/constant"
)

type Options struct {
	Name        string
	Socket      string
	MTU         uint32
	DeviceUsers map[string]string
}

type Listener struct{ options Options }

func New(options Options, tunnel C.Tunnel, additions ...inbound.Addition) (*Listener, error) {
	return nil, fmt.Errorf("OpenSurge packet listener requires the with_gvisor build tag")
}

func (l *Listener) Address() string    { return "unixgram:" + l.options.Socket }
func (l *Listener) RawAddress() string { return l.options.Socket }
func (l *Listener) Close() error       { return nil }
