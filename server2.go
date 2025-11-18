package main

import (
	"io"
	"log"
	"net"
)

func main() {
	// 1. Listen on port 7778
	listener, err := net.Listen("tcp", ":7778")
	if err != nil {
		log.Fatalf("Error Listening: %v", err)
	}
	// This defer runs when main returns (which is never, currently, but good practice)
	defer listener.Close()

	log.Println("Echo Server 2 listening on :7778")

	for {
		// 2. Accept a new connection
		clientConn, err := listener.Accept()
		if err != nil {
			log.Printf("Error Accepting Client Connection: %v", err)
			continue
		}

		log.Printf("Accepted connection from %s", clientConn.RemoteAddr())

		// 3. Handle it in a new goroutine
		// Note: We do NOT defer clientConn.Close() here in the loop!
		// It must be done inside the goroutine.
		go handleConnection(clientConn)
	}
}

func handleConnection(conn net.Conn) {
	// 4. Close the connection when this function exits
	defer conn.Close()

	// 5. The "Echo" Logic
	// io.Copy reads from 'src' and writes to 'dst' until EOF or error.
	// By passing 'conn' as both, we echo everything back.
	_, err := io.Copy(conn, conn)
	
	if err != nil {
		// "Use of closed network connection" is expected when the proxy hangs up
		log.Printf("Connection closed or error: %v", err)
	} else {
		log.Println("Connection closed cleanly.")
	}
}
