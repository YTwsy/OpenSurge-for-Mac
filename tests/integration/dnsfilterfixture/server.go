// Package dnsfilterfixture provides deterministic DNS replies for isolated core
// and host-network gates. It never forwards queries to public DNS.
package dnsfilterfixture

import (
	"encoding/binary"
	"io"
	"net"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/net/dns/dnsmessage"
)

type Server struct {
	tcp        net.Listener
	udp        net.PacketConn
	control    string
	wg         sync.WaitGroup
	once       sync.Once
	Queries    atomic.Int64
	TCPQueries atomic.Int64
}

func Start(address, control string) (*Server, error) {
	// Reserve the same port on both transports. Ephemeral TCP allocation alone
	// does not rule out an existing UDP owner; never evict it.
	var tcp net.Listener
	var udp net.PacketConn
	var err error
	_, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	for attempt := 0; attempt < 8; attempt++ {
		tcp, err = net.Listen("tcp", address)
		if err != nil {
			return nil, err
		}
		udp, err = net.ListenPacket("udp", tcp.Addr().String())
		if err == nil {
			break
		}
		_ = tcp.Close()
		if port != "0" {
			return nil, err
		}
	}
	if err != nil {
		return nil, err
	}
	s := &Server{tcp: tcp, udp: udp, control: control}
	s.wg.Add(2)
	go func() {
		defer s.wg.Done()
		buffer := make([]byte, 4096)
		for {
			n, peer, err := udp.ReadFrom(buffer)
			if err != nil {
				return
			}
			if answer := s.answer(buffer[:n], false); answer != nil {
				_, _ = udp.WriteTo(answer, peer)
			}
		}
	}()
	go func() {
		defer s.wg.Done()
		for {
			conn, err := tcp.Accept()
			if err != nil {
				return
			}
			s.wg.Add(1)
			go func() {
				defer s.wg.Done()
				defer conn.Close()
				_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
				var length [2]byte
				if _, err := io.ReadFull(conn, length[:]); err != nil {
					return
				}
				data := make([]byte, binary.BigEndian.Uint16(length[:]))
				if _, err := io.ReadFull(conn, data); err != nil {
					return
				}
				answer := s.answer(data, true)
				if answer != nil {
					_, _ = conn.Write(append([]byte{byte(len(answer) >> 8), byte(len(answer))}, answer...))
				}
			}()
		}
	}()
	return s, nil
}

func (s *Server) Address() string { return s.tcp.Addr().String() }
func (s *Server) Close()          { s.once.Do(func() { s.tcp.Close(); s.udp.Close(); s.wg.Wait() }) }

func (s *Server) answer(data []byte, tcp bool) []byte {
	var request dnsmessage.Message
	if request.Unpack(data) != nil || len(request.Questions) != 1 {
		return nil
	}
	s.Queries.Add(1)
	if tcp {
		s.TCPQueries.Add(1)
	}
	q := request.Questions[0]
	response := dnsmessage.Message{Header: dnsmessage.Header{ID: request.ID, Response: true, RecursionAvailable: true, RecursionDesired: request.RecursionDesired}, Questions: request.Questions}
	mode := strings.Split(strings.ToLower(q.Name.String()), ".")[0]
	if mode == "flip" {
		data, _ := os.ReadFile(s.control)
		mode = strings.TrimSpace(string(data))
	}
	ipv4 := [4]byte{203, 0, 113, 7}
	ipv6 := [16]byte{0x20, 0x01, 0x0d, 0xb8, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 7}
	switch mode {
	case "blocked", "block":
		response.RCode = dnsmessage.RCodeNameError
	case "refused":
		response.RCode = dnsmessage.RCodeRefused
	case "fail":
		response.RCode = dnsmessage.RCodeServerFailure
	case "empty":
	case "timeout":
		return nil
	case "truncated":
		if !tcp {
			response.Truncated = true
		} else {
			response.RCode = dnsmessage.RCodeRefused
		}
	default:
		if mode == "sink" {
			ipv4 = [4]byte{}
			ipv6 = [16]byte{}
		}
		if mode == "custom" {
			ipv4 = [4]byte{192, 0, 2, 99}
		}
		header := dnsmessage.ResourceHeader{Name: q.Name, Type: q.Type, Class: dnsmessage.ClassINET, TTL: 1}
		switch q.Type {
		case dnsmessage.TypeA:
			response.Answers = []dnsmessage.Resource{{Header: header, Body: &dnsmessage.AResource{A: ipv4}}}
		case dnsmessage.TypeAAAA:
			response.Answers = []dnsmessage.Resource{{Header: header, Body: &dnsmessage.AAAAResource{AAAA: ipv6}}}
		case dnsmessage.TypeTXT:
			response.Answers = []dnsmessage.Resource{{Header: header, Body: &dnsmessage.TXTResource{TXT: []string{"allowed"}}}}
		}
	}
	answer, _ := response.Pack()
	return answer
}

func Exchange(network, endpoint, domain string, qtype dnsmessage.Type) (dnsmessage.Message, error) {
	var response dnsmessage.Message
	name, err := dnsmessage.NewName(strings.TrimSuffix(domain, ".") + ".")
	if err != nil {
		return response, err
	}
	request := dnsmessage.Message{Header: dnsmessage.Header{ID: 123, RecursionDesired: true}, Questions: []dnsmessage.Question{{Name: name, Type: qtype, Class: dnsmessage.ClassINET}}}
	data, err := request.Pack()
	if err != nil {
		return response, err
	}
	conn, err := net.DialTimeout(network, endpoint, time.Second)
	if err != nil {
		return response, err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
	if network == "tcp" {
		data = append([]byte{byte(len(data) >> 8), byte(len(data))}, data...)
	}
	if _, err = conn.Write(data); err != nil {
		return response, err
	}
	buffer := make([]byte, 4096)
	if network == "tcp" {
		var length [2]byte
		if _, err = io.ReadFull(conn, length[:]); err != nil {
			return response, err
		}
		buffer = make([]byte, binary.BigEndian.Uint16(length[:]))
		_, err = io.ReadFull(conn, buffer)
	} else {
		var n int
		n, err = conn.Read(buffer)
		buffer = buffer[:n]
	}
	if err != nil {
		return response, err
	}
	err = response.Unpack(buffer)
	return response, err
}
