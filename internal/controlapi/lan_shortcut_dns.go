package controlapi

import (
	"encoding/binary"
	"io"
	"net"
	"net/netip"
	"strings"
	"time"

	"golang.org/x/net/dns/dnsmessage"
	"open-mihomo-gateway/internal/config"
)

// A non-recursive, loopback-only resolver for one management alias. No external
// queries, saved passwords or user-supplied DNS data enter the privileged helper.
func lanShortcutDNSReply(data []byte, binding lanSettings, available bool) []byte {
	var request dnsmessage.Message
	if err := request.Unpack(data); err != nil || request.Response {
		return nil
	}
	response := dnsmessage.Message{Header: dnsmessage.Header{ID: request.ID, Response: true, RecursionDesired: request.RecursionDesired}, Questions: request.Questions}
	if request.OpCode != 0 || len(request.Questions) != 1 {
		response.RCode = dnsmessage.RCodeFormatError
	} else {
		q := request.Questions[0]
		switch {
		case !strings.EqualFold(q.Name.String(), config.LANShortcutHost+".") || q.Class != dnsmessage.ClassINET:
			response.RCode = dnsmessage.RCodeRefused
		case !available:
			response.RCode = dnsmessage.RCodeNameError
		default:
			response.Authoritative = true
			if q.Type == dnsmessage.TypeA {
				ip, err := netip.ParseAddr(binding.IPv4)
				if err != nil || !ip.Is4() {
					return nil
				}
				response.Answers = []dnsmessage.Resource{{Header: dnsmessage.ResourceHeader{Name: q.Name, Type: dnsmessage.TypeA, Class: dnsmessage.ClassINET, TTL: 0}, Body: &dnsmessage.AResource{A: ip.As4()}}}
			}
			// AAAA and HTTPS/SVCB return NODATA: this HTTP shortcut is IPv4-only.
		}
	}
	reply, _ := response.Pack()
	return reply
}

func (s *lanShortcutService) serveDNSUDP(binding lanSettings, live func() bool) {
	defer s.Close()
	buffer := make([]byte, 4096)
	for {
		n, peer, err := s.udp.ReadFrom(buffer)
		if err != nil {
			return
		}
		if reply := lanShortcutDNSReply(buffer[:n], binding, live()); len(reply) > 0 {
			_, _ = s.udp.WriteTo(reply, peer)
		}
	}
}

func (s *lanShortcutService) serveDNSTCP(binding lanSettings, live func() bool) {
	defer s.Close()
	slots := make(chan struct{}, 16)
	for {
		conn, err := s.tcp.Accept()
		if err != nil {
			return
		}
		select {
		case slots <- struct{}{}:
		default:
			_ = conn.Close()
			continue
		}
		go func(conn net.Conn) {
			defer conn.Close()
			defer func() { <-slots }()
			_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
			var size [2]byte
			if _, err := io.ReadFull(conn, size[:]); err != nil {
				return
			}
			n := int(binary.BigEndian.Uint16(size[:]))
			if n < 12 || n > 4096 {
				return
			}
			data := make([]byte, n)
			if _, err := io.ReadFull(conn, data); err != nil {
				return
			}
			reply := lanShortcutDNSReply(data, binding, live())
			if len(reply) == 0 {
				return
			}
			binary.BigEndian.PutUint16(size[:], uint16(len(reply)))
			_, _ = conn.Write(append(size[:], reply...))
		}(conn)
	}
}
