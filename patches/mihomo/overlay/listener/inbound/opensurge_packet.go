package inbound

import (
	C "github.com/metacubex/mihomo/constant"
	"github.com/metacubex/mihomo/listener/opensurge_packet"
	"github.com/metacubex/mihomo/log"
)

type OpenSurgePacketOption struct {
	BaseOption
	Socket      string            `inbound:"socket"`
	MTU         uint32            `inbound:"mtu,omitempty"`
	DeviceUsers map[string]string `inbound:"device-users,omitempty"`
}

func (o OpenSurgePacketOption) Equal(config C.InboundConfig) bool {
	return optionToString(o) == optionToString(config)
}

type OpenSurgePacket struct {
	*Base
	config *OpenSurgePacketOption
	l      *opensurge_packet.Listener
}

func NewOpenSurgePacket(options *OpenSurgePacketOption) (*OpenSurgePacket, error) {
	base, err := NewBase(&options.BaseOption)
	if err != nil {
		return nil, err
	}
	return &OpenSurgePacket{Base: base, config: options}, nil
}

func (l *OpenSurgePacket) Config() C.InboundConfig { return l.config }

func (l *OpenSurgePacket) Address() string {
	if l.l == nil {
		return "unixgram:" + l.config.Socket
	}
	return l.l.Address()
}

func (l *OpenSurgePacket) RawAddress() string { return l.config.Socket }

func (l *OpenSurgePacket) Listen(tunnel C.Tunnel) error {
	listener, err := opensurge_packet.New(opensurge_packet.Options{
		Name: l.Name(), Socket: l.config.Socket, MTU: l.config.MTU, DeviceUsers: l.config.DeviceUsers,
	}, tunnel, l.Additions()...)
	if err != nil {
		return err
	}
	l.l = listener
	log.Infoln("OpenSurge packet[%s] proxy listening at: %s", l.Name(), l.Address())
	return nil
}

func (l *OpenSurgePacket) Close() error {
	if l.l == nil {
		return nil
	}
	return l.l.Close()
}

var _ C.InboundListener = (*OpenSurgePacket)(nil)
