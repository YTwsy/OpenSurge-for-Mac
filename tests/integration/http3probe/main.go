package main

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"flag"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"
)

const responseBody = "opensurge-http3-ok\n"

func main() {
	if len(os.Args) < 2 {
		usage()
	}

	var err error
	switch os.Args[1] {
	case "server":
		err = runServer(os.Args[2:])
	case "client":
		err = runClient(os.Args[2:])
	default:
		usage()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: opensurge-http3-probe <server|client> [flags]")
	os.Exit(2)
}

func runServer(args []string) error {
	flags := flag.NewFlagSet("server", flag.ContinueOnError)
	listen := flags.String("listen", "127.0.0.1:19096", "UDP listen address")
	logPath := flags.String("log", "", "request evidence log")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *logPath == "" {
		return errors.New("server log is required")
	}
	if err := os.MkdirAll(filepath.Dir(*logPath), 0o755); err != nil {
		return fmt.Errorf("create server log directory: %w", err)
	}

	certificate, err := generateCertificate()
	if err != nil {
		return err
	}
	packetConn, err := net.ListenPacket("udp4", *listen)
	if err != nil {
		return fmt.Errorf("listen for HTTP/3: %w", err)
	}
	defer packetConn.Close()

	handler := &evidenceHandler{logPath: *logPath}
	server := &http3.Server{
		TLSConfig: &tls.Config{
			Certificates: []tls.Certificate{certificate},
			MinVersion:   tls.VersionTLS13,
		},
		Handler: handler,
	}
	serveErr := make(chan error, 1)
	go func() {
		serveErr <- server.Serve(packetConn)
	}()

	fmt.Printf("READY protocol=h3 listen=%s\n", packetConn.LocalAddr())
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	select {
	case <-signals:
		_ = server.Close()
		return nil
	case err := <-serveErr:
		if errors.Is(err, http.ErrServerClosed) || errors.Is(err, net.ErrClosed) {
			return nil
		}
		return fmt.Errorf("serve HTTP/3: %w", err)
	}
}

type evidenceHandler struct {
	logPath string
	mu      sync.Mutex
}

func (h *evidenceHandler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	h.mu.Lock()
	file, err := os.OpenFile(h.logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err == nil {
		_, _ = fmt.Fprintf(file, "HTTP3 %s %s proto=%s host=%s remote=%s\n", request.Method, request.URL.RequestURI(), request.Proto, request.Host, request.RemoteAddr)
		_ = file.Close()
	}
	h.mu.Unlock()
	if err != nil {
		http.Error(writer, err.Error(), http.StatusInternalServerError)
		return
	}
	writer.Header().Set("X-OpenSurge-Protocol", "h3")
	writer.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(writer, responseBody)
}

func runClient(args []string) error {
	flags := flag.NewFlagSet("client", flag.ContinueOnError)
	requestURL := flags.String("url", "", "HTTPS URL used for SNI and the HTTP/3 request")
	targetIP := flags.String("address", "", "resolved IPv6 target address")
	sourceIP := flags.String("source", "", "downstream IPv6 source address")
	timeout := flags.Duration("timeout", 15*time.Second, "complete request timeout")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *requestURL == "" || *targetIP == "" || *sourceIP == "" {
		return errors.New("client url, address, and source are required")
	}

	parsedURL, err := url.Parse(*requestURL)
	if err != nil {
		return fmt.Errorf("parse request URL: %w", err)
	}
	if parsedURL.Scheme != "https" || parsedURL.Hostname() == "" {
		return errors.New("client URL must be an absolute https URL")
	}
	port := parsedURL.Port()
	if port == "" {
		port = "443"
	}
	target := net.JoinHostPort(strings.Trim(*targetIP, "[]"), port)
	remote, err := net.ResolveUDPAddr("udp6", target)
	if err != nil {
		return fmt.Errorf("resolve target address: %w", err)
	}
	localIP := net.ParseIP(strings.Trim(*sourceIP, "[]"))
	if localIP == nil || localIP.To4() != nil {
		return fmt.Errorf("invalid IPv6 source address %q", *sourceIP)
	}
	udpConn, err := net.ListenUDP("udp6", &net.UDPAddr{IP: localIP})
	if err != nil {
		return fmt.Errorf("bind HTTP/3 client to %s: %w", localIP, err)
	}
	defer udpConn.Close()
	quicTransport := &quic.Transport{Conn: udpConn}
	defer quicTransport.Close()

	transport := &http3.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true, // The fixture uses an ephemeral, process-local certificate.
			ServerName:         parsedURL.Hostname(),
			MinVersion:         tls.VersionTLS13,
		},
		QUICConfig: &quic.Config{
			HandshakeIdleTimeout: 8 * time.Second,
			MaxIdleTimeout:       12 * time.Second,
		},
		Dial: func(ctx context.Context, _ string, tlsConfig *tls.Config, quicConfig *quic.Config) (*quic.Conn, error) {
			return quicTransport.DialEarly(ctx, remote, tlsConfig, quicConfig)
		},
	}
	defer transport.Close()
	client := &http.Client{Transport: transport, Timeout: *timeout}
	response, err := client.Get(parsedURL.String())
	if err != nil {
		return fmt.Errorf("HTTP/3-only request failed: %w", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return fmt.Errorf("read HTTP/3 response: %w", err)
	}
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP/3 response status is %s", response.Status)
	}
	if response.ProtoMajor != 3 || response.Proto != "HTTP/3.0" {
		return fmt.Errorf("request unexpectedly used %s", response.Proto)
	}
	if response.Header.Get("X-OpenSurge-Protocol") != "h3" {
		return errors.New("HTTP/3 response marker is missing")
	}
	if string(body) != responseBody {
		return fmt.Errorf("unexpected HTTP/3 response body %q", body)
	}
	fmt.Printf("CLIENT_IPV6_HTTP3_OK protocol=%s source=%s target=%s host=%s path=%s\n", response.Proto, localIP, remote, parsedURL.Hostname(), parsedURL.EscapedPath())
	return nil
}

func generateCertificate() (tls.Certificate, error) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("generate certificate key: %w", err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "OpenSurge HTTP/3 Lab"},
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     []string{"*.opensurge.test"},
	}
	certificateDER, err := x509.CreateCertificate(rand.Reader, template, template, publicKey, privateKey)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("create certificate: %w", err)
	}
	return tls.Certificate{Certificate: [][]byte{certificateDER}, PrivateKey: privateKey}, nil
}
