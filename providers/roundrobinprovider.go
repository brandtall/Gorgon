package providers 
import(
	"log"
	"net"
	"sync"
	"time"
)

const HEALTH_CHECK_INTERVAL = 10 * time.Second
const HEALTH_CHECK_TIMEOUT = 1 * time.Second

func main() {}


type RRServerProvider struct {
	upstreamServers []string
	liveServers     []string
	mu              sync.Mutex
	index           int
}

func NewRRServerProvider(servers []string) *RRServerProvider {

	serverCopy := make([]string, len(servers))
	copy(serverCopy, servers)

	provider := &RRServerProvider{
		upstreamServers: serverCopy,
		liveServers:     append([]string{}, serverCopy...),
	}

	go provider.HealthChecks()

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

func (provider *RRServerProvider) HealthChecks() {
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
