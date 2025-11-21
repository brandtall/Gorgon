package main

import (
	"grogon/providers"
	"io"
	"log"
	"net"
	"sync"
	"time"
	"os"
	"os/signal"
	"syscall"
	"net/http"
	_ "net/http/pprof"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// --- Configuration ---
const IDLE_TIMEOUT = 10 * time.Minute
const DIAL_TIMEOUT = 2 * time.Second


// --- Globals ---

// bufferPool recycles 32KB buffers to reduce GC pressure.
var bufferPool = sync.Pool{
	New: func() interface{} {
		buffer := make([]byte, 32*1024)
		return &buffer
	},
}

type proxyMetrics struct {
	current_active_connections  prometheus.Gauge
	total_connections_handled prometheus.Counter
	total_connection_failures *prometheus.CounterVec
	connection_duration_seconds prometheus.Histogram
}

func NewMetrics(reg prometheus.Registerer) *proxyMetrics {
	metrics := &proxyMetrics{
	current_active_connections: prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "current_active_connections",
		Help: "Current Active Connections",
	}),
	total_connections_handled: prometheus.NewCounter(
			prometheus.CounterOpts{
				Name: "total_connections_handled",
				Help: "Total Number of Connections Handled",
			},
		),
	total_connection_failures: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "total_connection_failures",
				Help: "Total Number of Connections Failures.",
			},
			[]string{"reason"},
		),
	connection_duration_seconds: prometheus.NewHistogram(
        	prometheus.HistogramOpts{
				Name: "connection_duration_seconds",
				Help: "Duration of connections in seconds.",
				Buckets: prometheus.LinearBuckets(0.1, 0.1, 20),
			},
    ),
}
	reg.MustRegister(metrics.current_active_connections)
	reg.MustRegister(metrics.total_connections_handled)
	reg.MustRegister(metrics.total_connection_failures)
	reg.MustRegister(metrics.connection_duration_seconds)
	return metrics
}



type IServerProvider interface {
	Next() string
}

// --- Main Function ---
func main() {

	reg := prometheus.NewRegistry()

	metrics := NewMetrics(reg)

	http.Handle("/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{Registry: reg}))
	go func() {
		log.Println("Metrics server listening on :8081")
		if err := http.ListenAndServe(":8081", nil); err != nil {
			log.Fatalf("Metrics server error: %v", err)
		}
	}()

	go func() {
	log.Println("Starting pprof server on :6060")

	log.Println(http.ListenAndServe("localhost:6060", nil))
	}()

	upstreams := []string{
		"upstream1:7777",
		"upstream2:7777",
	}

	
	var serverProvider = providers.NewRRServerProvider(upstreams)

	var wg sync.WaitGroup


	listener, err := net.Listen("tcp", ":8080")
	if err != nil {
		log.Fatalf("Failed to start listener: %v", err)
	}
	defer listener.Close()

	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

		sig := <- sigChan
		log.Printf("Received signal: %v. Shutting down...", sig)

		listener.Close()
	}()


	log.Printf("Gorgon TCP Proxy listening on :8080\n")

	for {
		clientConn, err := listener.Accept()
		if err != nil {
			log.Printf("Failed to accept connection: %v", err)
			break
		}

		wg.Add(1)

		go handleConnection(clientConn, serverProvider, &wg, metrics)
	}
}

func handleConnection(clientConn net.Conn, provider IServerProvider, wg *sync.WaitGroup, metrics *proxyMetrics) {
	defer wg.Done()
	upstreamServer := provider.Next()
	if upstreamServer == "" {
		log.Println("No healthy upstream servers available.")
		clientConn.Close()
		return
	}

	log.Printf("Handling connection from %s, proxying to %s\n", clientConn.RemoteAddr(), upstreamServer)
	metrics.current_active_connections.Inc()
	defer metrics.current_active_connections.Dec()

	startTime := time.Now()
	defer func() {
        duration := time.Since(startTime)
        // This is what calculates your p99!
        metrics.connection_duration_seconds.Observe(duration.Seconds())
    }()

	metrics.total_connections_handled.Inc()


	serverConn, err := net.DialTimeout("tcp", upstreamServer, DIAL_TIMEOUT)
	if err != nil {
		metrics.total_connection_failures.WithLabelValues("dial_error").Inc()
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
