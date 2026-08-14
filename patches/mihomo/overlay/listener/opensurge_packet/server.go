//go:build with_gvisor

package opensurge_packet

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/metacubex/mihomo/adapter/inbound"
	C "github.com/metacubex/mihomo/constant"
	"github.com/metacubex/mihomo/listener/sing"
	"github.com/metacubex/mihomo/listener/sing_tun"
	"github.com/metacubex/mihomo/log"

	tun "github.com/metacubex/sing-tun"
	"github.com/metacubex/sing/common/buf"
	M "github.com/metacubex/sing/common/metadata"
	N "github.com/metacubex/sing/common/network"

	"github.com/metacubex/gvisor/pkg/buffer"
	"github.com/metacubex/gvisor/pkg/tcpip/header"
	"github.com/metacubex/gvisor/pkg/tcpip/link/channel"
	"github.com/metacubex/gvisor/pkg/tcpip/stack"
)

type Options struct {
	Name        string
	Socket      string
	MTU         uint32
	DeviceUsers map[string]string
}

type Listener struct {
	options  Options
	conn     *net.UnixConn
	tun      *packetTun
	tunStack tun.Stack
}

func New(options Options, tunnel C.Tunnel, additions ...inbound.Addition) (*Listener, error) {
	if options.Socket == "" {
		return nil, fmt.Errorf("socket is required")
	}
	if options.MTU < 1280 {
		options.MTU = 1500
	}
	if err := removeSocket(options.Socket); err != nil {
		return nil, err
	}
	addr := &net.UnixAddr{Name: options.Socket, Net: "unixgram"}
	conn, err := net.ListenUnixgram("unixgram", addr)
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(options.Socket, 0o600); err != nil {
		conn.Close()
		return nil, err
	}
	base, err := sing.NewListenerHandler(sing.ListenerConfig{Tunnel: tunnel, Type: C.TUN, Additions: additions})
	if err != nil {
		conn.Close()
		return nil, err
	}
	identities := newIdentityTable(options.DeviceUsers)
	packetTun := newPacketTun(conn, options.Socket+".broker", options.MTU, identities)
	inner := &sing_tun.ListenerHandler{ListenerHandler: base, DisableICMPForwarding: true}
	handler := &deviceHandler{inner: inner, identities: identities}
	stackOptions := tun.StackOptions{
		Context: context.Background(),
		Tun:     packetTun,
		TunOptions: tun.Options{
			Name: "opensurge-packet",
			MTU:  options.MTU,
		},
		UDPTimeout: 5 * time.Minute,
		Handler:    handler,
		Logger:     log.SingLogger,
	}
	tunStack, err := tun.NewStack("gvisor", stackOptions)
	if err != nil {
		conn.Close()
		return nil, err
	}
	if err := tunStack.Start(); err != nil {
		conn.Close()
		return nil, err
	}
	packetTun.Start()
	log.Infoln("OpenSurge packet listener ready: %s", options.Socket)
	return &Listener{options: options, conn: conn, tun: packetTun, tunStack: tunStack}, nil
}

func (l *Listener) Address() string    { return "unixgram:" + l.options.Socket }
func (l *Listener) RawAddress() string { return l.options.Socket }

func (l *Listener) Close() error {
	var result error
	if l.tunStack != nil {
		result = errors.Join(result, l.tunStack.Close())
	}
	if l.tun != nil {
		result = errors.Join(result, l.tun.Close())
	} else if l.conn != nil {
		result = errors.Join(result, l.conn.Close())
	}
	result = errors.Join(result, os.Remove(l.options.Socket))
	if errors.Is(result, os.ErrNotExist) {
		return nil
	}
	return result
}

func removeSocket(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSocket == 0 {
		return fmt.Errorf("refusing to replace non-socket path %s", path)
	}
	return os.Remove(path)
}

type identityTable struct {
	mu          sync.RWMutex
	byIPv6      map[netip.Addr]string
	deviceUsers map[string]string
}

func newIdentityTable(deviceUsers map[string]string) *identityTable {
	normalized := make(map[string]string, len(deviceUsers))
	for mac, user := range deviceUsers {
		normalized[strings.ToLower(mac)] = user
	}
	return &identityTable{byIPv6: map[netip.Addr]string{}, deviceUsers: normalized}
}

func (t *identityTable) Observe(ip netip.Addr, mac net.HardwareAddr) bool {
	macText := strings.ToLower(mac.String())
	t.mu.Lock()
	defer t.mu.Unlock()
	if current, ok := t.byIPv6[ip]; ok {
		if current != macText {
			log.Warnln("OpenSurge IPv6 identity conflict for %s: keep %s, reject %s", ip, current, macText)
			return false
		}
		return true
	}
	t.byIPv6[ip] = macText
	log.Infoln("OpenSurge IPv6 identity observed: source=%s mac=%s user=%s", ip, macText, t.deviceUsers[macText])
	return true
}

func (t *identityTable) User(ip netip.Addr) string {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.deviceUsers[t.byIPv6[ip]]
}

type deviceHandler struct {
	inner      *sing_tun.ListenerHandler
	identities *identityTable
}

func (h *deviceHandler) withIdentity(ctx context.Context, source M.Socksaddr) context.Context {
	if source.IsIP() {
		if user := h.identities.User(source.Addr); user != "" {
			return sing.WithAdditions(ctx, inbound.WithInUser(user))
		}
	}
	return ctx
}

func (h *deviceHandler) PrepareConnection(network string, source M.Socksaddr, destination M.Socksaddr, routeContext tun.DirectRouteContext, timeout time.Duration) (tun.DirectRouteDestination, error) {
	return h.inner.PrepareConnection(network, source, destination, routeContext, timeout)
}

func (h *deviceHandler) NewConnection(ctx context.Context, conn net.Conn, metadata M.Metadata) error {
	return h.inner.NewConnection(h.withIdentity(ctx, metadata.Source), conn, metadata)
}

func (h *deviceHandler) NewPacket(ctx context.Context, key netip.AddrPort, packet *buf.Buffer, metadata M.Metadata, init func(N.PacketConn) N.PacketWriter) {
	h.inner.NewPacket(h.withIdentity(ctx, metadata.Source), key, packet, metadata, init)
}

func (h *deviceHandler) NewError(ctx context.Context, err error) { h.inner.NewError(ctx, err) }

type packetTun struct {
	conn       *net.UnixConn
	peerPath   string
	mtu        uint32
	identities *identityTable
	endpoint   *channel.Endpoint
	ctx        context.Context
	cancel     context.CancelFunc
	closeOnce  sync.Once
	peerMu     sync.RWMutex
	peer       *net.UnixAddr
}

func newPacketTun(conn *net.UnixConn, peerPath string, mtu uint32, identities *identityTable) *packetTun {
	ctx, cancel := context.WithCancel(context.Background())
	return &packetTun{conn: conn, peerPath: peerPath, mtu: mtu, identities: identities, ctx: ctx, cancel: cancel}
}

func (t *packetTun) Start() {
	go t.readLoop()
	go t.writeLoop()
}

func (t *packetTun) Read([]byte) (int, error) { return 0, os.ErrInvalid }

func (t *packetTun) Write(packet []byte) (int, error) {
	if err := t.sendPacket(packet); err != nil {
		return 0, err
	}
	return len(packet), nil
}

func (t *packetTun) WritePacket(packet *stack.PacketBuffer) (int, error) {
	data := flattenPacket(packet)
	return t.Write(data)
}

func (t *packetTun) NewEndpoint() (stack.LinkEndpoint, stack.NICOptions, error) {
	t.endpoint = channel.New(1024, t.mtu, "")
	return t.endpoint, stack.NICOptions{}, nil
}

func (t *packetTun) Close() error {
	var err error
	t.closeOnce.Do(func() {
		t.cancel()
		if t.endpoint != nil {
			t.endpoint.Close()
		}
		err = t.conn.Close()
	})
	return err
}

func (t *packetTun) readLoop() {
	bufferBytes := make([]byte, int(t.mtu)+256)
	for {
		n, peer, err := t.conn.ReadFromUnix(bufferBytes)
		if err != nil {
			if !errors.Is(err, net.ErrClosed) {
				log.Warnln("OpenSurge packet listener read error: %v", err)
			}
			return
		}
		if peer == nil || peer.Name != t.peerPath {
			continue
		}
		message, err := decodeMessage(bufferBytes[:n])
		if err != nil {
			continue
		}
		t.peerMu.Lock()
		t.peer = peer
		t.peerMu.Unlock()
		if message.typ != messageInbound || len(message.packet) < header.IPv6MinimumSize || message.packet[0]>>4 != 6 {
			continue
		}
		source := netip.AddrFrom16([16]byte(message.packet[8:24]))
		if !t.identities.Observe(source, message.mac) {
			continue
		}
		packet := stack.NewPacketBuffer(stack.PacketBufferOptions{Payload: buffer.MakeWithData(message.packet)})
		t.endpoint.InjectInbound(header.IPv6ProtocolNumber, packet)
		packet.DecRef()
	}
}

func (t *packetTun) writeLoop() {
	for {
		packet := t.endpoint.ReadContext(t.ctx)
		if packet == nil {
			return
		}
		data := flattenPacket(packet)
		packet.DecRef()
		if err := t.sendPacket(data); err != nil && !errors.Is(err, net.ErrClosed) {
			log.Warnln("OpenSurge packet listener write error: %v", err)
		}
	}
}

func (t *packetTun) sendPacket(packet []byte) error {
	if len(packet) < header.IPv6MinimumSize || packet[0]>>4 != 6 {
		return nil
	}
	t.peerMu.RLock()
	peer := t.peer
	t.peerMu.RUnlock()
	if peer == nil {
		return nil
	}
	message, err := encodeMessage(messageOutbound, nil, packet)
	if err != nil {
		return err
	}
	_, err = t.conn.WriteToUnix(message, peer)
	return err
}

func flattenPacket(packet *stack.PacketBuffer) []byte {
	views := packet.AsSlices()
	length := 0
	for _, view := range views {
		length += len(view)
	}
	data := make([]byte, 0, length)
	for _, view := range views {
		data = append(data, view...)
	}
	return data
}

var _ tun.GVisorTun = (*packetTun)(nil)
var _ tun.Handler = (*deviceHandler)(nil)
