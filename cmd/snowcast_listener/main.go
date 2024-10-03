package main

import (
	"fmt"
	"log"
	"net"
	"os"
)

const (
	MaxMessageSize = 1500
)

func main() {
	if len(os.Args) != 2 {
		fmt.Printf("Usage:  %s <port>\n", os.Args[0])
		os.Exit(1)
	}

	port := os.Args[1]
	listenString := fmt.Sprintf(":%s", port)

	listenAddr, err := net.ResolveUDPAddr("udp4", listenString)
	if err != nil {
		log.Panicln("Error resolving address:  ", err)
	}

	conn, err := net.ListenUDP("udp4", listenAddr)
	if err != nil {
		log.Panicln("Could not bind to UDP port: ", err)
	}

	for {
		buffer := make([]byte, MaxMessageSize)
		bytesRead, _, err := conn.ReadFromUDP(buffer)
		if err != nil {
			log.Panicln("Error reading from UDP socket ", err)
		}

		os.Stdout.Write(buffer[:bytesRead])
	}
}
