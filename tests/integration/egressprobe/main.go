package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

func main() {
	originAddr := flag.String("origin", "", "origin HTTP listen address")
	proxyAddr := flag.String("proxy", "", "HTTP CONNECT proxy listen address")
	logDir := flag.String("log-dir", "", "directory for origin.log and proxy.log")
	flag.Parse()

	if *originAddr == "" || *proxyAddr == "" || *logDir == "" {
		fmt.Fprintln(os.Stderr, "origin, proxy, and log-dir are required")
		os.Exit(2)
	}
	if err := os.MkdirAll(*logDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "create log dir: %v\n", err)
		os.Exit(1)
	}

	originListener, err := net.Listen("tcp", *originAddr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "listen origin: %v\n", err)
		os.Exit(1)
	}
	proxyListener, err := net.Listen("tcp", *proxyAddr)
	if err != nil {
		originListener.Close()
		fmt.Fprintf(os.Stderr, "listen proxy: %v\n", err)
		os.Exit(1)
	}

	originLog := filepath.Join(*logDir, "origin.log")
	proxyLog := filepath.Join(*logDir, "proxy.log")
	originServer := &http.Server{Handler: &originHandler{logPath: originLog}}
	proxyServer := &http.Server{Handler: &connectProxyHandler{logPath: proxyLog}}

	var serverWG sync.WaitGroup
	serverWG.Add(2)
	go serve(&serverWG, originServer, originListener, "origin")
	go serve(&serverWG, proxyServer, proxyListener, "proxy")

	fmt.Println("READY")

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	<-signals

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = originServer.Shutdown(ctx)
	_ = proxyServer.Shutdown(ctx)
	serverWG.Wait()
}

func serve(wg *sync.WaitGroup, server *http.Server, listener net.Listener, name string) {
	defer wg.Done()
	err := server.Serve(listener)
	if err != nil && err != http.ErrServerClosed {
		fmt.Fprintf(os.Stderr, "%s server: %v\n", name, err)
	}
}

type originHandler struct {
	logPath string
	mu      sync.Mutex
}

func (h *originHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.appendLog(fmt.Sprintf("%s %s", r.Method, r.URL.RequestURI()))
	w.Header().Set("Content-Type", "text/plain")
	_, _ = io.WriteString(w, "origin-ok\n")
}

func (h *originHandler) appendLog(line string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	appendLine(h.logPath, line)
}

type connectProxyHandler struct {
	logPath string
	mu      sync.Mutex
}

func (h *connectProxyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodConnect {
		h.appendLog(fmt.Sprintf("%s %s", r.Method, r.RequestURI))
		http.Error(w, "CONNECT required", http.StatusMethodNotAllowed)
		return
	}

	target := r.Host
	if target == "" {
		target = r.RequestURI
	}
	if !strings.Contains(target, ":") {
		http.Error(w, "CONNECT target must include a port", http.StatusBadRequest)
		return
	}
	h.appendLog("CONNECT " + target)

	upstream, err := net.DialTimeout("tcp", target, 5*time.Second)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}

	hijacker, ok := w.(http.Hijacker)
	if !ok {
		upstream.Close()
		http.Error(w, "hijacking unsupported", http.StatusInternalServerError)
		return
	}
	client, buffered, err := hijacker.Hijack()
	if err != nil {
		upstream.Close()
		return
	}
	defer client.Close()
	defer upstream.Close()

	if _, err := io.WriteString(client, "HTTP/1.1 200 Connection Established\r\n\r\n"); err != nil {
		return
	}
	if buffered.Reader.Buffered() > 0 {
		if _, err := io.CopyN(upstream, buffered.Reader, int64(buffered.Reader.Buffered())); err != nil {
			return
		}
	}

	done := make(chan struct{}, 2)
	go tunnel(done, upstream, client)
	go tunnel(done, client, upstream)
	<-done
}

func (h *connectProxyHandler) appendLog(line string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	appendLine(h.logPath, line)
}

func tunnel(done chan<- struct{}, dst net.Conn, src net.Conn) {
	_, _ = io.Copy(dst, src)
	done <- struct{}{}
}

func appendLine(path string, line string) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "append %s: %v\n", path, err)
		return
	}
	defer file.Close()
	_, _ = fmt.Fprintln(file, line)
}
