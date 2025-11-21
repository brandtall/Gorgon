package main


import(
	"net"
	"log"
	"time"
	"sync"
)



func main() {
	var wg sync.WaitGroup
	for i := 0; i < 10000; i++  {
		wg.Add(1)
		go func() {
			defer wg.Done()
			log.Print("New Connection Starting")
			// Inside the goroutine:
			proxyConn, err := net.Dial("tcp", "grogon-proxy:8080")
			if err != nil {
  		  log.Printf("Connect error: %v", err)
    		return // <-- MUST RETURN
			}
			defer proxyConn.Close()

		// Create a buffer with some data
		data := []byte("Hello Gorgon Payload")
		proxyConn.Write(data)

		// Read the response (echo)
		readBuf := make([]byte, 1024)
		proxyConn.Read(readBuf)

		time.Sleep(10 * time.Second)		}()
	}
	wg.Wait()
}
