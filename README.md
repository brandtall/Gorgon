# Gorgon: High-Performance TCP Proxy Server

A high throughput, resilient TCP load balancer built with Go routines.

## Benchmarks

**Sustaining \~80,000 QPS on a single node.**
<img width="2238" height="1186" alt="image" src="https://github.com/user-attachments/assets/fad2eb8b-4fbe-40d9-b8b1-236a3daf56e7" /> 

### Load Test Logs

```bash
load-tester-1  | 2025/11/22 09:35:06 Current Throughput: 79718 QPS
load-tester-1  | 2025/11/22 09:35:07 Current Throughput: 80343 QPS
load-tester-1  | 2025/11/22 09:35:08 Current Throughput: 82564 QPS
load-tester-1  | 2025/11/22 09:35:09 Current Throughput: 79643 QPS
load-tester-1  | 2025/11/22 09:35:10 Current Throughput: 74912 QPS
load-tester-1  | 2025/11/22 09:35:11 Current Throughput: 78448 QPS
```

## Architecture

### Concurrency Model

Normally, using a regular "1 connection per OS thread" model is problematic as we can easily run out of memory. By default, a thread gets allocated 1MB of stack memory on Windows and 8MB on Linux. For our case of 10k concurrent connections, we are talking about **10GB to 80GB of memory** just for stacks.

To circumvent this, proxy servers usually use event loops (non blocking I/O) to handle N connections per OS thread, but this complicates the logic. However, with Go, Goroutines are an out of the box solution. They are managed by the Go runtime and default to a **2KB stack**, making them extremely lightweight.

For this use case, Go is more than powerful enough to handle 10k+ connections on modest hardware.

### Memory Optimization

Another bottleneck hit when handling a high number of concurrent connections is Garbage Collection, particularly when we are continuously allocating buffers for copying data.

This is where `sync.Pool` comes in. Instead of continuously allocating new memory buffers and counting on the GC to reclaim them (which causes CPU spikes), we use a shared pool of buffers.

The real power of `sync.Pool` is that it allocates **separate pools per CPU core**. This means all cores are working at the same time and they (mostly) don't fight over locks. Sometimes one core might steal buffers from another's pool, but it is rare. One catch is that Garbage Collection will eventually kick in and release all buffers in the pool, which causes a small spike in allocations immediately afterwards, but the trade off is worth it.

### Resilience

To ensure upstream servers are live, we have **active health checks** that run periodically. They temporarily remove dead servers from rotation and automatically return them when they recover.

Currently, we have a Round Robin load balancer for this POC (which can be replaced with Least Connections easily).

We also handle **graceful shutdown** through syscalls (`SIGINT`/`SIGTERM`) and `sync.WaitGroup`, ensuring no active client connections are dropped during a deployment.

## Getting Started

### Prerequisites

  * **Docker & Docker Compose**
  * Linux/WSL2/macOS

### 1\. Installation

Clone the repository:

```bash
git clone https://github.com/brandtall/Gorgon.git
cd Gorgon
```

### 2\. Unlock OS Limits

If you want to test high concurrency (10k+ connections), you need to increase your file descriptor limits, or the OS will kill the connections.

```bash
ulimit -n 65535
```

### 3\. Spin Up the Stack

We use Docker Compose to spin up the Proxy, the Upstream Echo Servers, Prometheus, Grafana, and the Load Generator all at once.

```bash
# Build the containers and start them in the background
docker-compose up --build -d
```

### 4\. View Metrics

Once the stack is running, you can view the observability dashboards:

  * **Prometheus:** `http://localhost:9090`
  * **Grafana:** `http://localhost:3000` (Login: `admin`/`admin`)
  * **Pprof (Profiling):** `http://localhost:6060/debug/pprof`

### 5\. Run the Load Test

To see the throughput spike (like in the benchmarks above), verify the load tester logs:

```bash
docker-compose logs -f load-tester
```

To scale the load test up (simulate a distributed attack):

```bash
docker-compose up -d --scale load-tester=5 --no-recreate load-tester
```

