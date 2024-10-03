package protocol

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"time"
)

const (
	Prompt       = "> "
	ServerPrompt = ""

	MessageTypeHello          = 0
	MessageTypeSetStation     = 1
	MessageTypeWelcome        = 2
	MessageTypeAnnounce       = 3
	MessageTypeInvalidCommand = 4
)

type Hello struct {
	CommandType uint8
	UdpPort     uint16
}

type Welcome struct {
	ReplyType   uint8
	NumStations uint16
}

type SetStation struct {
	CommandType   uint8
	StationNumber uint16
}

type Announce struct {
	ReplyType    uint8
	SongNameSize uint8
	SongName     []byte
}

type InvalidCommand struct {
	ReplyType       uint8
	ReplyStringSize uint8
	ReplyString     []byte
}

func (m *Hello) MarshalHello() ([]byte, error) {
	buf := new(bytes.Buffer)

	// Add Message Type
	err := binary.Write(buf, binary.BigEndian, m.CommandType)
	if err != nil {
		panic(err)
	}

	// Add Message Content
	err = binary.Write(buf, binary.BigEndian, m.UdpPort)
	if err != nil {
		panic(err)
	}

	return buf.Bytes(), nil
}

func (m *Welcome) MarshalWelcome() ([]byte, error) {
	buf := new(bytes.Buffer)

	// Add Message Type
	err := binary.Write(buf, binary.BigEndian, m.ReplyType)
	if err != nil {
		panic(err)
	}

	// Add Message Content
	err = binary.Write(buf, binary.BigEndian, m.NumStations)
	if err != nil {
		panic(err)
	}

	return buf.Bytes(), nil
}

func (m *SetStation) MarshalSetStation() ([]byte, error) {
	buf := new(bytes.Buffer)

	// Add Message Type
	err := binary.Write(buf, binary.BigEndian, m.CommandType)
	if err != nil {
		panic(err)
	}

	// Add Message Content
	err = binary.Write(buf, binary.BigEndian, m.StationNumber)
	if err != nil {
		panic(err)
	}

	return buf.Bytes(), nil
}

func (m *Announce) MarshalAnnounce() ([]byte, error) {
	/*
		type Announce struct {
		ReplyType    uint8
		SongNameSize uint8
		SongName     []byte
	}*/
	buf := new(bytes.Buffer)

	// Add Message Type
	err := binary.Write(buf, binary.BigEndian, m.ReplyType)
	if err != nil {
		panic(err)
	}

	// Add Message Content
	err = binary.Write(buf, binary.BigEndian, m.SongNameSize)
	if err != nil {
		panic(err)
	}

	// Add Song Name
	err = binary.Write(buf, binary.BigEndian, m.SongName)
	if err != nil {
		panic(err)
	}

	return buf.Bytes(), nil
}

func (m *InvalidCommand) MarshalInvalidCommand() ([]byte, error) {
	buf := new(bytes.Buffer)

	// Add Message Type
	err := binary.Write(buf, binary.BigEndian, m.ReplyType)
	if err != nil {
		panic(err)
	}

	// Add Message Content
	err = binary.Write(buf, binary.BigEndian, m.ReplyStringSize)
	if err != nil {
		panic(err)
	}

	// Add Song Name
	err = binary.Write(buf, binary.BigEndian, m.ReplyString)
	if err != nil {
		panic(err)
	}

	return buf.Bytes(), nil
}

func ReadHelloMessage(conn net.Conn) (*Hello, error) {
	buffer := make([]byte, 2)
	_, err := io.ReadFull(conn, buffer)
	if err != nil {
		return nil, err
	}

	conn.SetReadDeadline(time.Time{})

	helloMessage := &Hello{
		CommandType: MessageTypeHello,
		UdpPort:     binary.BigEndian.Uint16(buffer),
	}

	return helloMessage, nil
}

func ReadWelcomeMessage(conn net.Conn) (*Welcome, error) {
	buffer := make([]byte, 2)
	_, err := io.ReadFull(conn, buffer)
	if err != nil {
		panic(err)
	}

	conn.SetReadDeadline(time.Time{})

	welcomeMessage := &Welcome{
		ReplyType:   MessageTypeWelcome,
		NumStations: binary.BigEndian.Uint16(buffer),
	}

	return welcomeMessage, nil
}

func ReadSetStationMessage(conn net.Conn) (*SetStation, error) {
	buffer := make([]byte, 2)
	_, err := io.ReadFull(conn, buffer)
	if err != nil {
		return nil, err
	}

	conn.SetReadDeadline(time.Time{})

	setStationMessage := &SetStation{
		CommandType:   MessageTypeSetStation,
		StationNumber: binary.BigEndian.Uint16(buffer),
	}

	return setStationMessage, nil
}

func ReadAnnounceMessage(conn net.Conn, receivedAnnounce chan bool) (setStationMessage *Announce, err error) {
	songNameSize := make([]byte, 1)
	_, err = io.ReadFull(conn, songNameSize)
	if err != nil {
		panic(err)
	}

	songName := make([]byte, uint8(songNameSize[0]))
	_, err = io.ReadFull(conn, songName)
	if err != nil {
		panic(err)
	}

	conn.SetReadDeadline(time.Time{})

	annonuceMessage := &Announce{
		ReplyType:    MessageTypeAnnounce,
		SongNameSize: songNameSize[0],
		SongName:     songName,
	}

	return annonuceMessage, nil
}

func ReadInvalidCommandMessage(conn net.Conn) (*InvalidCommand, error) {
	replyStringSize := make([]byte, 1)
	_, err := io.ReadFull(conn, replyStringSize)
	if err != nil {
		panic(err)
	}

	replyString := make([]byte, uint8(replyStringSize[0]))
	_, err = io.ReadFull(conn, replyString)
	if err != nil {
		panic(err)
	}

	conn.SetReadDeadline(time.Time{})

	invalidCommandMessage := &InvalidCommand{
		ReplyType:       MessageTypeInvalidCommand,
		ReplyStringSize: replyStringSize[0],
		ReplyString:     replyString,
	}

	fmt.Printf("error: INVALID_COMMAND_REPLY: %s\n", string(replyString))
	fmt.Printf("Server has closed the connection.\n")

	return invalidCommandMessage, nil
}
