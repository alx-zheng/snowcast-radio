package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"snowcast/pkg/protocol"
	"snowcast/pkg/util"
	"strconv"
	"strings"
	"sync"
	"time"
)

type ClientInfo struct {
	Conn                           net.Conn
	ConnClosed                     bool
	ClientAddress                  string
	ClientIP                       string
	ClientPort                     int
	ListenerPort                   int
	NumHelloMessagesSent           int
	InitialHelloReceivedChannel    chan bool
	InitialHelloReceivedInTimeBool chan bool
	FirstHelloReceived             bool
}

type ServerInfo struct {
	NumStations               int
	MusicFiles                []string
	ClientAddrToStationMap    map[string]int
	ClientAddrToClientInfoMap map[string]*ClientInfo
	ChannelToClientMap        map[int][]string
	ChannelToFileMap          map[int]string
	ListenConn                *net.TCPListener
}

var serverInfoLock sync.Mutex
var debug = false

func main() {
	tcpPort := os.Args[1]
	files := os.Args[2:]

	if len(files) < 1 {
		fmt.Println("usage: snowcast_server <tcpport> <file0> [file 1] [file 2] ...")
		os.Exit(0)
	}

	// ResolveTCP Address
	addrToUse := fmt.Sprintf("%s:%s", "127.0.0.1", tcpPort)
	addr, err := net.ResolveTCPAddr("tcp4", addrToUse)
	if err != nil {
		panic(err)
	}

	// Create listening connection for TCP handshake
	listenConn, err := net.ListenTCP("tcp4", addr)
	if err != nil {
		panic(err)
	}

	serverInfo := &ServerInfo{
		NumStations:               len(files),
		MusicFiles:                files,
		ClientAddrToClientInfoMap: make(map[string]*ClientInfo),
		ClientAddrToStationMap:    make(map[string]int),
		ChannelToFileMap:          initializeStationToFileMap(files),
		ChannelToClientMap:        initializeStationToClientsMap(files),
		ListenConn:                listenConn, // The Servers TCP Listener
	}

	serverInfo.startStationBroadcasts()
	go listenForNewClientConnections(serverInfo)

	// REPL
	prompt := protocol.ServerPrompt
	input := os.Stdin
	output := os.Stdout
	scanner := bufio.NewScanner(input)

	io.WriteString(output, prompt)
	for scanner.Scan() {
		tokens := strings.Split(scanner.Text(), " ")
		if tokens[0] == "q" {
			serverInfo.closeAllClientConnections()
			os.Exit(0)
		} else if len(tokens) == 2 && tokens[0] == "p" {
			serverInfo.printStationToFileMapToFile(tokens[1])
		} else if tokens[0] == "p" {
			serverInfo.printStationToFileMap()
		} else if tokens[0] == "c" {
			serverInfo.printStationToClientMap()
		} else if tokens[0] == "m" {
			serverInfo.printClientAddrToChannelMap()
		} else if tokens[0] == "n" {
			serverInfo.printClientAddrToClientInfoMap()
		}
		io.WriteString(output, prompt)
	}
}

func (ServerInfo *ServerInfo) startStationBroadcasts() {
	if debug {
		fmt.Println("Starting All Stations Broadcasts...")
	}
	for channel := 0; channel < ServerInfo.NumStations; channel++ {
		go broadcastToClients(channel, ServerInfo)
	}
}

func broadcastToClients(stationNumber int, serverInfo *ServerInfo) {
	audioFile, err := os.Open(serverInfo.MusicFiles[stationNumber])
	if err != nil {
		log.Panicln("Error opening file: ", err)
	}
	defer audioFile.Close()

	for {
		serverInfoLock.Lock()
		LatestServerInfo := *serverInfo
		serverInfoLock.Unlock()

		// Figure out sound data to send
		bytesToSend := make([]byte, 1024)
		_, err := audioFile.Read(bytesToSend)
		if err != nil {
			if err == io.EOF {
				audioFile.Seek(0, io.SeekStart) // When EOF is reached, seek back to the start

				// Send an Announce message to all clients listening to this station when song repeats.
				clientAddrs := LatestServerInfo.ChannelToClientMap[stationNumber]
				for _, clientAddr := range clientAddrs {
					clientInfo := LatestServerInfo.ClientAddrToClientInfoMap[clientAddr]

					announceMessage := protocol.Announce{
						ReplyType:    protocol.MessageTypeAnnounce,
						SongNameSize: uint8(len(serverInfo.ChannelToFileMap[stationNumber])),
						SongName:     []byte(serverInfo.ChannelToFileMap[stationNumber]),
					}

					bytesToSend, err := announceMessage.MarshalAnnounce()
					if err != nil {
						panic(err)
					}

					_, err = clientInfo.Conn.Write(bytesToSend)
					if err != nil {
						panic(err)
					}

					if debug {
						fmt.Printf("[SERVER -> CLIENT] Announced because song repeated to: %s\n", serverInfo.ChannelToFileMap[stationNumber])
					}
				}
				continue // Start reading from the beginning again
			} else {
				if debug {
					fmt.Println("Error reading file:", err)
				}
				break
			}
		}

		// Send song bytes to clients
		clientAddrs := LatestServerInfo.ChannelToClientMap[stationNumber]
		for _, clientAddr := range clientAddrs {
			clientInfo := LatestServerInfo.ClientAddrToClientInfoMap[clientAddr]

			listenerUDPAddressString := fmt.Sprintf("%s:%d", clientInfo.ClientIP, clientInfo.ListenerPort)
			remoteListenerAddr, err := net.ResolveUDPAddr("udp4", listenerUDPAddressString)
			if err != nil {
				log.Panicln("Error resolving address:  ", err)
			}

			conn, err := net.DialUDP("udp4", nil, remoteListenerAddr)
			if err != nil {
				log.Panicln("Dial: ", err)
			}

			_, err = conn.Write([]byte(bytesToSend))
			if err != nil {
				log.Panicln("Error writing to socket: ", err)
			}
		}

		time.Sleep(time.Duration(62)*time.Millisecond + time.Duration(500)*time.Microsecond)
	}
}

func listenForNewClientConnections(serverInfo *ServerInfo) {
	for {
		clientConn, err := serverInfo.ListenConn.AcceptTCP()
		if err != nil {
			panic(err)
		}

		fmt.Printf("session id %s: new client connected; expecting HELLO\n", clientConn.RemoteAddr().String())

		clientInfo := &ClientInfo{
			Conn:                           clientConn,
			ClientIP:                       strings.Split(clientConn.RemoteAddr().String(), ":")[0],
			ClientPort:                     clientConn.RemoteAddr().(*net.TCPAddr).Port,
			NumHelloMessagesSent:           0,
			InitialHelloReceivedChannel:    make(chan bool, 1),
			InitialHelloReceivedInTimeBool: make(chan bool, 1),
			FirstHelloReceived:             false,
			ConnClosed:                     false,
		}

		// For each client, spawn a goroutine to handle the client
		if debug {
			fmt.Printf("Starting new goroutine to handle client with address: %s\n", clientInfo.Conn.RemoteAddr())
		}
		go handleClient(clientInfo, serverInfo)

		// Ensure that server receive client's first Hello message within 100ms
		select {
		case <-clientInfo.InitialHelloReceivedChannel:
			clientInfo.InitialHelloReceivedInTimeBool <- true
		case <-time.After(100 * time.Millisecond):
			fmt.Println("[TIMEOUT] Did not receive initial Hello Message in Time. Closing Connection for " + clientInfo.Conn.RemoteAddr().String())
			errorString := "Did not receive your initial Hello Message in Time. Closing Our Connection."
			invalidCommandMessage := protocol.InvalidCommand{
				ReplyType:       protocol.MessageTypeInvalidCommand,
				ReplyStringSize: uint8(len(errorString)),
				ReplyString:     []byte(errorString),
			}
			bytesToSend, err := invalidCommandMessage.MarshalInvalidCommand()
			if err != nil {
				panic(err)
			}

			if !clientInfo.ConnClosed { // If the connection is not already closed, send the error message
				_, err = clientInfo.Conn.Write(bytesToSend)
				if err != nil {
					panic(err)
				}
			}
			clientInfo.ConnClosed = true
			clientInfo.Conn.Close()
			clientInfo.InitialHelloReceivedInTimeBool <- false
		}

	}
}

func handleClient(clientInfo *ClientInfo, serverInfo *ServerInfo) {
	defer clientInfo.Conn.Close()
	defer serverInfo.removeClientFromServerState(clientInfo)

	for {
		buffer := make([]byte, 1)
		_, err := io.ReadFull(clientInfo.Conn, buffer)
		if err != nil {
			if debug {
				fmt.Printf("Client %s Disconnected. Exiting Gracefully...\n", clientInfo.ClientAddress)
			}
			return
		}

		// Set Deadline to receive rest of Message within 100ms.
		clientInfo.Conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))

		replyType := buffer[0]
		if replyType == protocol.MessageTypeHello {
			helloMessage, err := protocol.ReadHelloMessage(clientInfo.Conn)
			if err != nil {
				if errors.Is(err, os.ErrDeadlineExceeded) {
					fmt.Println("[ERROR]: Timeout(100ms) on receiving Hello Message: ", err)
					errorString := "[ERROR]: Timeout(100ms) on receiving Hello Message: "
					invalidCommandMessage := protocol.InvalidCommand{
						ReplyType:       protocol.MessageTypeInvalidCommand,
						ReplyStringSize: uint8(len(errorString)),
						ReplyString:     []byte(errorString),
					}
					bytesToSend, err := invalidCommandMessage.MarshalInvalidCommand()
					if err != nil {
						panic(err)
					}

					if !clientInfo.ConnClosed { // If the connection is not already closed, send the error message
						_, err = clientInfo.Conn.Write(bytesToSend)
						if err != nil {
							panic(err)
						}
					}
					clientInfo.ConnClosed = true
				} else {
					fmt.Println("[ERROR]: ", err)
				}

				return
			}

			if !clientInfo.FirstHelloReceived { // If we haven't received the first Hello message yet, send into clientInfo.InitialHelloReceivedChannel <- true to indicate that we received first Hello.
				clientInfo.InitialHelloReceivedChannel <- true

				clientInfo.FirstHelloReceived = <-clientInfo.InitialHelloReceivedInTimeBool // Receive from clientInfo.InitialHelloReceivedInTimeBool to check if we received the first Hello message in time.
				if !clientInfo.FirstHelloReceived {                                         // If we didn't receive in time, terminate this handleClient() thread.
					return
				}
			}

			listenerPort := helloMessage.UdpPort
			clientInfo.ListenerPort = int(listenerPort)
			clientInfo.ClientAddress = clientInfo.ClientIP + ":" + strconv.Itoa(clientInfo.ListenerPort)
			clientInfo.NumHelloMessagesSent++

			// If the client has already sent a HelloMessage, send an error message and terminate the connection
			if clientInfo.NumHelloMessagesSent > 1 {
				fmt.Println("more than 1 print")
				errorString := "Received More Than One Hello Message"
				invalidCommandMessage := protocol.InvalidCommand{
					ReplyType:       protocol.MessageTypeInvalidCommand,
					ReplyStringSize: uint8(len(errorString)),
					ReplyString:     []byte(errorString),
				}
				bytesToSend, err := invalidCommandMessage.MarshalInvalidCommand()
				if err != nil {
					panic(err)
				}

				_, err = clientInfo.Conn.Write(bytesToSend)
				if err != nil {
					panic(err)
				}
				return
			}

			// Send a welcome response to the client
			welcomeMessage := protocol.Welcome{
				ReplyType:   protocol.MessageTypeWelcome,
				NumStations: uint16(serverInfo.NumStations),
			}

			bytesToSend, err := welcomeMessage.MarshalWelcome()
			if err != nil {
				panic(err)
			}

			_, err = clientInfo.Conn.Write(bytesToSend)
			if err != nil {
				panic(err)
			}

			serverInfo.ClientAddrToClientInfoMap[clientInfo.ClientAddress] = clientInfo

			fmt.Printf("session id %s: HELLO received; sending WELCOME, expecting SET_STATION\n", clientInfo.Conn.RemoteAddr().String())

		} else if replyType == protocol.MessageTypeSetStation {
			setStationMessage, err := protocol.ReadSetStationMessage(clientInfo.Conn)
			if err != nil {
				if errors.Is(err, os.ErrDeadlineExceeded) {
					fmt.Println("[ERROR]: Timeout(100ms) on receiving Set Station Message: ", err)
					errorString := "[ERROR]: Timeout(100ms) on receiving Set Station Message: "
					invalidCommandMessage := protocol.InvalidCommand{
						ReplyType:       protocol.MessageTypeInvalidCommand,
						ReplyStringSize: uint8(len(errorString)),
						ReplyString:     []byte(errorString),
					}
					bytesToSend, err := invalidCommandMessage.MarshalInvalidCommand()
					if err != nil {
						panic(err)
					}

					_, err = clientInfo.Conn.Write(bytesToSend)
					if err != nil {
						panic(err)
					}
				} else {
					fmt.Println("[ERROR]: ", err)
				}
				return
			}

			if clientInfo.NumHelloMessagesSent == 0 {
				errorString := "Received SetStationMessage Before HelloMessage"
				invalidCommandMessage := protocol.InvalidCommand{
					ReplyType:       protocol.MessageTypeInvalidCommand,
					ReplyStringSize: uint8(len(errorString)),
					ReplyString:     []byte(errorString),
				}
				bytesToSend, err := invalidCommandMessage.MarshalInvalidCommand()
				if err != nil {
					panic(err)
				}

				_, err = clientInfo.Conn.Write(bytesToSend)
				if err != nil {
					panic(err)
				}
				return
			}

			newStationNumber := int(setStationMessage.StationNumber)
			if newStationNumber < 0 || newStationNumber > serverInfo.NumStations-1 {
				errorString := "Invalid Station Number"
				invalidCommandMessage := protocol.InvalidCommand{
					ReplyType:       protocol.MessageTypeInvalidCommand,
					ReplyStringSize: uint8(len(errorString)),
					ReplyString:     []byte(errorString),
				}
				bytesToSend, err := invalidCommandMessage.MarshalInvalidCommand()
				if err != nil {
					panic(err)
				}

				_, err = clientInfo.Conn.Write(bytesToSend)
				if err != nil {
					panic(err)
				}

				return
			}

			fmt.Printf("session id %s: received SET_STATION to station %d\n", clientInfo.ClientAddress, newStationNumber)

			announceMessage := protocol.Announce{
				ReplyType:    protocol.MessageTypeAnnounce,
				SongNameSize: uint8(len(serverInfo.ChannelToFileMap[newStationNumber])),
				SongName:     []byte(serverInfo.ChannelToFileMap[newStationNumber]),
			}

			bytesToSend, err := announceMessage.MarshalAnnounce()
			if err != nil {
				panic(err)
			}

			if debug {
				fmt.Printf("[SERVER -> CLIENT] Announcing song: %s\n", serverInfo.ChannelToFileMap[newStationNumber])
			}
			_, err = clientInfo.Conn.Write(bytesToSend)
			if err != nil {
				panic(err)
			}

			go func() {
				// Check if the client is already listening to the station
				// If the client is already listening to the station, do nothing
				_, ok := serverInfo.ClientAddrToStationMap[clientInfo.ClientAddress]
				if ok && serverInfo.ClientAddrToStationMap[clientInfo.ClientAddress] == newStationNumber {
					if debug {
						fmt.Println("Client already listening to this station")
					}
					return
				} else { // Client is subscribing to a new station or subscribing for the first time
					// If the client was already subscribed to a channel, remove the client from the old stationStationNumber->[]clients map

					// Getting ready to update server state, lock mutex
					serverInfoLock.Lock()

					oldStationNumber := serverInfo.ClientAddrToStationMap[clientInfo.ClientAddress]
					indexToRemove := util.StringIndexOf(serverInfo.ChannelToClientMap[oldStationNumber], clientInfo.ClientAddress)
					if indexToRemove != -1 {
						serverInfo.ChannelToClientMap[oldStationNumber] = util.RemoveIndex(serverInfo.ChannelToClientMap[oldStationNumber], indexToRemove)
					}

					// Add the client to the new stationStationNumber->[]clients map
					serverInfo.ChannelToClientMap[newStationNumber] = append(serverInfo.ChannelToClientMap[newStationNumber], clientInfo.ClientAddress)
					serverInfo.ClientAddrToStationMap[clientInfo.ClientAddress] = newStationNumber
					serverInfoLock.Unlock()
				}
			}()

		} else {
			errorString := "Did not receive HelloMessage or SetStationMessage"
			invalidCommandMessage := protocol.InvalidCommand{
				ReplyType:       protocol.MessageTypeInvalidCommand,
				ReplyStringSize: uint8(len(errorString)),
				ReplyString:     []byte(errorString),
			}
			bytesToSend, err := invalidCommandMessage.MarshalInvalidCommand()
			if err != nil {
				panic(err)
			}

			_, err = clientInfo.Conn.Write(bytesToSend)
			if err != nil {
				panic(err)
			}

			return
		}
	}
}

func (ServerInfo *ServerInfo) removeClientFromServerState(clientInfo *ClientInfo) (*ClientInfo, error) {
	/*
		Remove a client's footprint on the server state.
		Return the ClientInfo that was removed.
	*/
	clientAddress := clientInfo.ClientAddress

	if debug {
		fmt.Println("Cleaning up server state for client ->", clientAddress)
	}

	delete(ServerInfo.ClientAddrToStationMap, clientAddress)
	delete(ServerInfo.ClientAddrToClientInfoMap, clientAddress)

	for channel := 0; channel < len(ServerInfo.ChannelToClientMap); channel++ {
		index := util.StringIndexOf(ServerInfo.ChannelToClientMap[channel], clientAddress)
		if index != -1 {
			ServerInfo.ChannelToClientMap[channel] = util.RemoveIndex(ServerInfo.ChannelToClientMap[channel], index)
		}
	}

	return clientInfo, nil
}

func (ServerInfo *ServerInfo) closeAllClientConnections() {
	/*
		For all clients on server, close their connections.
	*/

	for _, clientInfo := range ServerInfo.ClientAddrToClientInfoMap {
		clientInfo.Conn.Close()
	}
}

func initializeStationToClientsMap(files []string) map[int][]string {
	/*
		For each channel (station), initialize an empty list of clients.
	*/
	fileToClientsMap := make(map[int][]string)

	for i := 0; i < len(files); i++ {
		fileToClientsMap[i] = make([]string, 0)
	}

	return fileToClientsMap
}

func initializeStationToFileMap(files []string) map[int]string {
	/*
		For each channel (station), map the channel to the corresponding file.
		This mapping is corresponding to the order of the files passed in as arguments.
		First file corresponds to channel 0, second file corresponds to channel 1, etc.
	*/
	channelToFileMap := make(map[int]string)

	for i, file := range files {
		channelToFileMap[i] = file
	}

	return channelToFileMap
}

func (ServerInfo *ServerInfo) printStationToFileMap() {
	for channel := 0; channel < len(ServerInfo.ChannelToFileMap); channel++ {
		fmt.Printf("%d,%s,", channel, ServerInfo.ChannelToFileMap[channel])
		for i, clientAddr := range ServerInfo.ChannelToClientMap[channel] {
			fmt.Print(clientAddr)
			if i < len(ServerInfo.ChannelToClientMap[channel])-1 {
				fmt.Print(",")
			}
		}
		fmt.Println()
	}
}

func (ServerInfo *ServerInfo) printStationToFileMapToFile(filename string) {
	file, err := os.Create(filename)
	if err != nil {
		fmt.Println("Error creating file:", err)
		return
	}
	defer file.Close()

	for channel := 0; channel < len(ServerInfo.ChannelToFileMap); channel++ {
		fmt.Fprintf(file, "%d,%s,", channel, ServerInfo.ChannelToFileMap[channel])
		for i, v := range ServerInfo.ChannelToClientMap[channel] {
			fmt.Fprint(file, v)
			if i < len(ServerInfo.ChannelToClientMap[channel])-1 {
				fmt.Fprint(file, ",")
			}
		}
		fmt.Fprint(file, "\n")
	}
}

func (ServerInfo *ServerInfo) printStationToClientMap() {
	for channel := 0; channel < len(ServerInfo.ChannelToClientMap); channel++ {
		fmt.Println("Channel", channel)
		fmt.Println("Clients", ServerInfo.ChannelToClientMap[channel])
	}
}

func (ServerInfo *ServerInfo) printClientAddrToChannelMap() {
	for client, channel := range ServerInfo.ClientAddrToStationMap {
		fmt.Printf("client: %s -> channel: %d\n", client, channel)
	}
}

func (ServerInfo *ServerInfo) printClientAddrToClientInfoMap() {
	for client, clientInfo := range ServerInfo.ClientAddrToClientInfoMap {
		fmt.Printf("client: %s -> clientInfo: %v\n", client, clientInfo)
	}
}
