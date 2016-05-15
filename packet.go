package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

const (
	RRQ  = uint16(1)
	WRQ  = uint16(2)
	DATA = uint16(3)
	ACK  = uint16(4)
	ERR  = uint16(5)
)

type request struct {
	opcode   uint16
	filename string
	mode     string
}

// TODO: Fix getting the mode, looks kinda ugly
func parseRequest(b []byte) (*request, error) {
	if len(b) < 4 {
		return nil, fmt.Errorf("Not a valid tftp packet")
	}
	opcode, remaining, ok := parsePacket(b)
	if !ok {
		return nil, fmt.Errorf("Not a valid tftp packet")
	}
	fname, remainingMode, ok := nullTerm(remaining)
	if !ok {
		return nil, fmt.Errorf("Not a valid tftp packet")
	}
	mode, _, ok := nullTerm(remainingMode[1:])
	if !ok {
		return nil, fmt.Errorf("Not a valid tftp packet")
	}
	return &request{opcode, string(fname), string(mode)}, nil
}

// TODO: improve the parser to return not only the remaining part, but the actual parts of the packet
//       rrq/wrq - filename - mode
// parse checks if p is a valid tftp packet.
// It returns the opcode, the remaining payload and a boolean which
// indicates if its a valid packet
func parsePacket(p []byte) (opcode uint16, remaining []byte, ok bool) {
	if len(p) < 4 {
		return 0, nil, false
	}
	opcode = binary.BigEndian.Uint16(p[:2])
	switch opcode {
	case RRQ, WRQ, DATA, ACK, ERR:
		remaining = p[2:]
		return opcode, remaining, true
	default:
		return 0, nil, false
	}
}

func parseError(b []byte) (errCode uint16, errMsg string, ok bool) {
	errCode = binary.BigEndian.Uint16(b[:2])
	msg, _, ok := nullTerm(b[2:])
	if !ok {
		return 0, "", false
	}
	return errCode, string(msg), true
}

func ackPacket(id uint16) []byte {
	b := make([]byte, 4)
	binary.BigEndian.PutUint16(b[:2], ACK)
	binary.BigEndian.PutUint16(b[2:], id)
	return b
}

func dataPacket(id uint16, data []byte) []byte {
	b := make([]byte, len(data)+4)
	binary.BigEndian.PutUint16(b[:2], DATA)
	binary.BigEndian.PutUint16(b[2:4], id)
	copy(b[4:], data)
	return b
}

func errorPacket(errcode uint16, msg string) []byte {
	b := make([]byte, len(msg)+5)
	binary.BigEndian.PutUint16(b[:2], ERR)
	binary.BigEndian.PutUint16(b[2:4], errcode)
	copy(b[4:], msg)
	return b
}

// nullTerm returns a before and remaining byte slice of the first instance
// of the 0 byte in the tftp packet and a bool indicating the presence of a 0 byte
func nullTerm(p []byte) (before, after []byte, ok bool) {
	index := bytes.IndexByte(p, 0)
	if index == -1 {
		return nil, nil, false
	}
	return p[:index], p[index:], true
}
