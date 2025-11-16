package main

import (
	"io"
	"log"
	"net"
	"sync"
	"time"
)

// --- Configuration ---
const IDLE_TIMEOUT = 10 * time.Minute
const DIAL_TIMEOUT = 2 * time.Second
const HEALTH_CHECK_INTERVAL = 10 * time.Second
const HEALTH_CHECK_TIMEOUT = 1 * time.Second

// --- Globals ---

// bufferPool recycles 32KB buffers to reduce GC pressure.
var bufferPool = sync.Pool{
	New: func() interface{} {
		buffer := make([]byte, 32*1024)
		return &buffer
	},
}

type IServerProvider interface {
	Next() string
}

// --- Main Function ---
func main() {
	upstreams := []string{
		"google.com:80",
		"httpbin.org:80",
		"bad-host-does-not-exist:80",
	}

	var serverProvider = newRRServerProvider(upstreams)

	listener, err := net.Listen("tcp", ":8080")
	if err != nil {
		log.Fatalf("Failed to start listener: %v", err)
	}
	defer listener.Close()

	log.Printf("Gorgon TCP Proxy listening on :8080\n")

	for {
		clientConn, err := listener.Accept()
		if err != nil {
			log.Printf("Failed to accept connection: %v", err)
			continue
		}

		go handleConnection(clientConn, serverProvider)
	}
}

func handleConnection(clientConn net.Conn, provider IServerProvider) {
	upstreamServer := provider.Next()
	if upstreamServer == "" {
		log.Println("No healthy upstream servers available.")
		clientConn.Close()
		return
	}

	log.Printf("Handling connection from %s, proxying to %s\n", clientConn.RemoteAddr(), upstreamServer)

	serverConn, err := net.DialTimeout("tcp", upstreamServer, DIAL_TIMEOUT)
	if err != nil {
		log.Printf("Failed to dial upstream '%s': %v", upstreamServer, err)
		clientConn.Close()
		return
	}

	defer clientConn.Close()
	defer serverConn.Close()

	go copyWithIdleTimeout(clientConn, serverConn, IDLE_TIMEOUT)
	copyWithIdleTimeout(serverConn, clientConn, IDLE_TIMEOUT)

	log.Printf("Closing connection from %s (upstream %s)\n", clientConn.RemoteAddr(), upstreamServer)
}

func copyWithIdleTimeout(destination net.Conn, source net.Conn, timeout time.Duration) {
	bufferPtr := bufferPool.Get().(*[]byte)
	defer bufferPool.Put(bufferPtr)
	buffer := *bufferPtr

	for {
		source.SetReadDeadline(time.Now().Add(timeout))

		numBytes, err := source.Read(buffer)
		if err != nil {
			if err != io.EOF {
			}
			return
		}

		destination.SetWriteDeadline(time.Now().Add(timeout))
		_, err = destination.Write(buffer[:numBytes])
		if err != nil {
			log.Printf("Write error: %v", err)
			return
		}
	}
}

// --- Round Robin Provider (Thread-Safe) ---

// RRServerProvider implements IServerProvider with
// round-robin logic and active health checking.
type RRServerProvider struct {
	upstreamServers []string
	liveServers     []string
	mu              sync.Mutex
	index           int
}

func newRRServerProvider(servers []string) *RRServerProvider {

	serverCopy := make([]string, len(servers))
	copy(serverCopy, servers)

	provider := &RRServerProvider{
		upstreamServers: serverCopy,
		liveServers:     append([]string{}, serverCopy...),
	}

	go provider.healthChecks()

	return provider
}

func (provider *RRServerProvider) Next() string {

	provider.mu.Lock()
	defer provider.mu.Unlock()

	if len(provider.liveServers) == 0 {
		return ""
	}

	selectedServer := provider.liveServers[provider.index]
	provider.index = (provider.index + 1) % len(provider.liveServers)

	return selectedServer
}

func (provider *RRServerProvider) healthChecks() {
	ticker := time.NewTicker(HEALTH_CHECK_INTERVAL)
	defer ticker.Stop()

	for range ticker.C {
		log.Println("Running health checks...")

		var newLiveServers []string
		for _, server := range provider.upstreamServers {
			conn, err := net.DialTimeout("tcp", server, HEALTH_CHECK_TIMEOUT)
			if err == nil {
				newLiveServers = append(newLiveServers, server)
				conn.Close()
			} else {
				log.Printf("Health check failed for %s: %v", server, err)
			}
		}

		provider.mu.Lock()
		provider.liveServers = newLiveServers
		provider.index = 0
		provider.mu.Unlock()

		log.Printf("Health check complete. Live servers: %v", provider.liveServers)
	}
}
