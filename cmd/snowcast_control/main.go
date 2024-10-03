package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"snowcast/pkg/protocol"
	"strconv"
	"strings"
	"time"
)

type MessageCounts struct {
	numSetStations int
	numAnnounce    int
	numHello       int
	numWelcome     int
	numInvalid     int
}

var receivedFirstWelcome = make(chan bool)
var receivedFirstWelcomeBoolean = false

func main() {
	if len(os.Args) != 4 {
		fmt.Printf("Usage:  %s <server_name> <server_port> <udp_name>\n", os.Args[0])
		os.Exit(0)
	}

	serverIP := os.Args[1]
	serverPort := os.Args[2]
	listenerPort := os.Args[3]

	messageCounts := &MessageCounts{
		numSetStations: 0,
	}

	// Create client socket
	addrToUse := fmt.Sprintf("%s:%s", serverIP, serverPort)
	conn, err := net.Dial("tcp4", addrToUse)
	if err != nil {
		panic(err)
	}

	go ReceiveServerMessages(conn, messageCounts)

	// Convert listenerPort from string to uint16
	listenerPortUint, err := strconv.ParseUint(listenerPort, 10, 16)
	if err != nil {
		panic(err)
	}

	// Create hello message to send to server
	helloMessage := protocol.Hello{
		CommandType: protocol.MessageTypeHello,
		UdpPort:     uint16(listenerPortUint),
	}

	// Marshal Hello Message
	bytesToSend, err := helloMessage.MarshalHello()
	if err != nil {
		panic(err)
	}

	// Send hello message to server
	_, err = conn.Write(bytesToSend)
	if err != nil {
		panic(err)
	}
	messageCounts.numHello++

	// // Make sure we received a WelcomeMessage within 100ms before proceeding to the Client REPL.
	select {
	case <-receivedFirstWelcome:
		receivedFirstWelcomeBoolean = true
	case <-time.After(100 * time.Millisecond):
		fmt.Println("[TIMEOUT]: Timeout(100ms) on receiving first Welcome Message")
		conn.Close()
		os.Exit(0)
	}

	prompt := protocol.Prompt
	input := os.Stdin
	output := os.Stdout
	scanner := bufio.NewScanner(input)

	io.WriteString(output, prompt)
	// REPL
	for scanner.Scan() {
		tokens := strings.Split(scanner.Text(), " ")
		if tokens[0] == "q" {
			os.Exit(0)
		}

		command, err := strconv.Atoi(tokens[0])
		if err != nil {
			io.WriteString(output, "Channel must be a numeric character\n")
			io.WriteString(output, prompt)
			continue
		}

		setStationMessage := protocol.SetStation{
			CommandType:   uint8(protocol.MessageTypeSetStation),
			StationNumber: uint16(command),
		}

		// Marshal SetStation Message
		bytesToSend, err := setStationMessage.MarshalSetStation()
		if err != nil {
			panic(err)
		}

		// Send SetStation message to server
		messageCounts.numSetStations++
		_, err = conn.Write(bytesToSend)
		if err != nil {
			panic(err)
		}

		fmt.Println("Waiting for an announce...")
	}
	io.WriteString(output, "\n")
}

func ReceiveServerMessages(conn net.Conn, messageCounts *MessageCounts) {
	for {
		buffer := make([]byte, 1)
		_, err := conn.Read(buffer)
		conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
		if err != nil {
			panic(err)
		}

		replyType := buffer[0]
		if replyType == protocol.MessageTypeWelcome {
			welcomeMessage, err := protocol.ReadWelcomeMessage(conn)
			if err != nil {
				if errors.Is(err, os.ErrDeadlineExceeded) {
					fmt.Println("[ERROR]: Timeout(100ms) on receiving Welcome Message: ", err)
				} else {
					fmt.Println("[ERROR]: ", err)
				}
				conn.Close()
				os.Exit(0)
			}

			messageCounts.numWelcome++
			if messageCounts.numWelcome > messageCounts.numHello || messageCounts.numWelcome > 1 { // Client should ever only receive one welcome message. Client should not receive a welcome message before sending a HellloMessage.
				conn.Close()
				os.Exit(0)
			}

			fmt.Print(conn.RemoteAddr().(*net.TCPAddr).IP.String() + " -> " + conn.RemoteAddr().(*net.TCPAddr).IP.String() + "\n")
			fmt.Print("Type in a number to set the station we're listening to to that number.\n")
			fmt.Print("Type in 'q' or press CTRL+C to quit.\n")
			fmt.Printf("The server has %d stations.\n", welcomeMessage.NumStations)

			// Make sure we only pass into this receivedFirstWelcome channel once.
			if !receivedFirstWelcomeBoolean {
				receivedFirstWelcome <- true
			}

		} else if replyType == protocol.MessageTypeAnnounce {
			receivedAnnounce := make(chan bool)
			announceMessage, err := protocol.ReadAnnounceMessage(conn, receivedAnnounce)
			if err != nil {
				if errors.Is(err, os.ErrDeadlineExceeded) {
					fmt.Println("[ERROR]: Timeout(100ms) on receiving Announce Message: ", err)
				} else {
					fmt.Println("[ERROR]: ", err)
				}
				conn.Close()
				os.Exit(0)
			}

			messageCounts.numAnnounce++
			if messageCounts.numSetStations == 0 { // Announces should always be less than or equal to set stations.
				conn.Close()
				os.Exit(0)
			}

			fmt.Printf("New song announced: %s\n", string(announceMessage.SongName))
			fmt.Print(protocol.Prompt)

		} else if replyType == protocol.MessageTypeInvalidCommand {
			messageCounts.numInvalid++
			protocol.ReadInvalidCommandMessage(conn)
			conn.Close()
			os.Exit(0)

		} else {
			fmt.Println("Invalid Reply Type: Must be Welcome, Announce, or InvalidCommand")
			conn.Close()
			os.Exit(0)

		}
	}
}
