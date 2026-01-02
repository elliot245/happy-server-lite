package socketio

import (
	"encoding/json"
	"errors"
	"strconv"
	"strings"
)

type enginePacketType byte

const (
	engineOpen    enginePacketType = '0'
	engineClose   enginePacketType = '1'
	enginePing    enginePacketType = '2'
	enginePong    enginePacketType = '3'
	engineMessage enginePacketType = '4'
)

type socketPacketType byte

const (
	socketConnect socketPacketType = '0'
	socketEvent   socketPacketType = '2'
	socketAck     socketPacketType = '3'
)

func parseOptionalNamespace(s string) (namespace string, rest string) {
	if !strings.HasPrefix(s, "/") {
		return "/", s
	}
	comma := strings.IndexByte(s, ',')
	if comma == -1 {
		return "/", s
	}
	return s[:comma], s[comma+1:]
}

func parseOptionalIDPrefix(s string) (id *int, rest string) {
	i := 0
	for i < len(s) {
		c := s[i]
		if c < '0' || c > '9' {
			break
		}
		i++
	}
	if i == 0 {
		return nil, s
	}
	v, err := strconv.Atoi(s[:i])
	if err != nil {
		return nil, s
	}
	return &v, s[i:]
}

type socketEventPacket struct {
	Namespace string
	ID        *int
	Event     string
	Args      []json.RawMessage
}

func parseSocketEventPacket(payload string) (socketEventPacket, error) {
	if payload == "" {
		return socketEventPacket{}, errors.New("empty payload")
	}
	if payload[0] != byte(socketEvent) {
		return socketEventPacket{}, errors.New("not an event packet")
	}

	ns, rest := parseOptionalNamespace(payload[1:])
	id, rest := parseOptionalIDPrefix(rest)
	if !strings.HasPrefix(rest, "[") {
		return socketEventPacket{}, errors.New("invalid event payload")
	}

	var arr []json.RawMessage
	if err := json.Unmarshal([]byte(rest), &arr); err != nil {
		return socketEventPacket{}, err
	}
	if len(arr) == 0 {
		return socketEventPacket{}, errors.New("missing event name")
	}
	var eventName string
	if err := json.Unmarshal(arr[0], &eventName); err != nil {
		return socketEventPacket{}, errors.New("invalid event name")
	}

	return socketEventPacket{Namespace: ns, ID: id, Event: eventName, Args: arr[1:]}, nil
}

type socketAckPacket struct {
	Namespace string
	ID        int
	Args      []json.RawMessage
}

func parseSocketAckPacket(payload string) (socketAckPacket, error) {
	if payload == "" {
		return socketAckPacket{}, errors.New("empty payload")
	}
	if payload[0] != byte(socketAck) {
		return socketAckPacket{}, errors.New("not an ack packet")
	}

	ns, rest := parseOptionalNamespace(payload[1:])
	id, rest := parseOptionalIDPrefix(rest)
	if id == nil {
		return socketAckPacket{}, errors.New("missing ack id")
	}
	if !strings.HasPrefix(rest, "[") {
		return socketAckPacket{}, errors.New("invalid ack payload")
	}

	var arr []json.RawMessage
	if err := json.Unmarshal([]byte(rest), &arr); err != nil {
		return socketAckPacket{}, err
	}
	return socketAckPacket{Namespace: ns, ID: *id, Args: arr}, nil
}

func buildSocketEventPacket(namespace string, id *int, event string, args ...any) (string, error) {
	arr := make([]any, 0, 1+len(args))
	arr = append(arr, event)
	arr = append(arr, args...)
	data, err := json.Marshal(arr)
	if err != nil {
		return "", err
	}

	var b strings.Builder
	b.WriteByte(byte(socketEvent))
	if namespace != "" && namespace != "/" {
		b.WriteString(namespace)
		b.WriteByte(',')
	}
	if id != nil {
		b.WriteString(strconv.Itoa(*id))
	}
	b.Write(data)
	return b.String(), nil
}

func buildSocketConnectPacket(namespace string, sid string) (string, error) {
	data, err := json.Marshal(map[string]string{"sid": sid})
	if err != nil {
		return "", err
	}

	var b strings.Builder
	b.WriteByte(byte(socketConnect))
	if namespace != "" && namespace != "/" {
		b.WriteString(namespace)
		b.WriteByte(',')
	}
	b.Write(data)
	return b.String(), nil
}

func buildSocketAckPacket(namespace string, id int, args ...any) (string, error) {
	if args == nil {
		args = make([]any, 0)
	}
	data, err := json.Marshal(args)
	if err != nil {
		return "", err
	}

	var b strings.Builder
	b.WriteByte(byte(socketAck))
	if namespace != "" && namespace != "/" {
		b.WriteString(namespace)
		b.WriteByte(',')
	}
	b.WriteString(strconv.Itoa(id))
	b.Write(data)
	return b.String(), nil
}
