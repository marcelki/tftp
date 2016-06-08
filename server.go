package main

import (
	"bufio"
	"encoding/binary"
	"flag"
	"fmt"
	"github.com/marcelki/tftp/netascii"
	"io"
	"log"
	"net"
	"os"
	"syscall"
	"time"
)

const (
	TRIES = 3
)

var (
	port = flag.String("port", ":69", "The port to listen on")
	dir  = flag.String("dir", "", "The directory to serve")
)

func main() {
	flag.Parse()

	log.Fatalln(ListenAndServe(*port))
}

type session struct {
	addr net.Addr
	req  *request
}

func ListenAndServe(port string) error {
	pconn, err := net.ListenPacket("udp", port)
	if err != nil {
		return err
	}
	session := &session{}
	return session.Serve(pconn)
}

func (s *session) Serve(pconn net.PacketConn) error {
	defer pconn.Close()
	buf := make([]byte, 516)
	for {
		n, addr, err := pconn.ReadFrom(buf)
		if err != nil {
			if nErr, ok := err.(net.Error); ok && nErr.Temporary() {
				continue
			}
			return err
		}
		request, err := parseRequest(buf[:n])
		if err != nil {
			log.Printf("Request is not valid: %v.", err)
			continue
		}
		s.req = request
		s.addr = addr

		switch s.req.opcode {
		case RRQ:
			if *dir != "" {
				s.req.filename = *dir + "/" + s.req.filename
			}
			go s.ReadRequest()
		case WRQ:
			if *dir != "" {
				s.req.filename = *dir + "/" + s.req.filename
			}
			go s.WriteRequest()
		default:
			// TODO: received packet is incorrect
		}
	}
}

func (s *session) ReadRequest() {
	addr := s.addr
	conn, err := net.Dial("udp", addr.String())
	if err != nil {
		log.Printf("RRQ: Could not connect to %s: %s\n", addr.String(), err)
		return
	}
	defer conn.Close()

	if exists := fileExists(s.req.filename); !exists {
		err := s.sendError(conn, uint16(1), "File not found")
		if err != nil {
			log.Printf("RRQ: Error sending error packet to %v.\n", addr)
		}
		log.Printf("RRQ: Requested file %v does not exist\n", s.req.filename)
		return
	}
	fd, err := os.Open(s.req.filename)
	if err != nil {
		err = s.sendError(conn, uint16(0), "Not defined error: Could not open the file descriptor")
		if err != nil {
			log.Printf("RRQ: Error sending error packet to %v.\n", addr)
		}
		log.Printf("RRQ: Could not open the %s file descriptor for reading: %s \n", s.req.filename, err)
		return

	}
	// initial ack id
	id := uint16(1)
	// check if we get a performance gain, when not reusing the slice/array
	data := make([]byte, 512)
	for {
		n, err := io.ReadFull(fd, data)
		if err != nil && err != io.ErrUnexpectedEOF {
			return
		}
		err = s.sendData(conn, id, data[:n])
		if err != nil {
			// TOOD: logging
			return
		}
		id++
	}
}

func (s *session) WriteRequest() {
	addr := s.addr
	conn, err := net.Dial("udp", addr.String())
	if err != nil {
		log.Printf("WRQ: Could not connect to %s: %s\n", addr.String(), err)
		return
	}
	defer conn.Close()

	if exists := fileExists(s.req.filename); exists {
		err := s.sendError(conn, uint16(6), "File already exists.")
		if err != nil {
			log.Printf("WRQ: Error sending error packet to %v.\n", addr)
		}
		log.Printf("WRQ: File %v does already exist\n", s.req.filename)
		return
	}

	// TODO: use different permission for the file
	fd, err := os.OpenFile(s.req.filename, os.O_CREATE|os.O_WRONLY, 0777)
	if err != nil {
		if e, ok := err.(*os.PathError); ok && e.Err == syscall.ENOSPC {
			err = s.sendError(conn, uint16(3), "Disk full or allocation exceeded")
			if err != nil {
				log.Printf("WRQ: Error sending error packet to %v.\n", addr)
			}
			log.Printf("WRQ: Not enough space to open the %s file descriptor\n", s.req.filename)
			return
		}
		err = s.sendError(conn, uint16(0), "Not defined error: Could not open the file descriptor")
		if err != nil {
			log.Printf("WRQ: Error sending error packet to %v.\n", addr)
		}
		log.Printf("WRQ: Could not open the %s file descriptor: %s \n", s.req.filename, err)
		return
	}
	defer fd.Close()

	bw := bufio.NewWriter(fd)
	id := uint16(0)
	for {
		// TODO: what happens when the client sends an error
		// we will never flush the writer ?!
		data, err := s.sendAck(conn, id, false)
		if err != nil {
			log.Printf("WRQ: Error sending ack packet to %v.\n", addr)
			return
		}
		id++
		if s.req.mode == "octet" {
			_, err = bw.Write(data)
		} else if s.req.mode == "netascii" {
			_, err = netascii.WriteTo(data, bw)
		} else {
			// TODO: logging
			fmt.Println("Mode not implemented yet!")
			return
		}
		if err != nil {
			log.Printf("WRQ: %v", err)
			return
		}
		if len(data) < 512 {
			s.sendAck(conn, id, true)
			bw.Flush()
			return
		}
	}
}

// sendAck sends a ack packet and returns the next data packet
// TODO: write a new routine to handle the write request, it works for now, but it isnt simple and elegant
func (s *session) sendAck(conn net.Conn, id uint16, last bool) (data []byte, err error) {
	p := ackPacket(id)
Tx:
	for try := 0; try < TRIES; try++ {
		conn.Write(p)
		conn.SetReadDeadline(time.Now().Add(time.Second))

		// TODO: search for a better alternative handling the last packet / termination of the session
		if last {
			return nil, err
		}

		recv := make([]byte, 516)
		for {
			n, err := conn.Read(recv)
			if err != nil {
				if nErr, ok := err.(net.Error); ok && nErr.Timeout() {
					continue Tx
				}
				return nil, err
			}
			opcode, remaining, ok := parsePacket(recv[:n])
			if !ok {
				continue
			}
			//blockid := remaining[:2]
			data = remaining[2:]
			switch opcode {
			case DATA:
				return data, nil
			case ERR:
				// TOOD: do something when error send
			}
		}
	}
	return nil, fmt.Errorf("timed outw waiting for the next data packet\n")
}

func (s *session) sendData(conn net.Conn, id uint16, data []byte) error {
	p := dataPacket(id, data)
Tx:
	for try := 0; try < TRIES; try++ {
		conn.Write(p)
		conn.SetReadDeadline(time.Now().Add(time.Second))

		// TODO: consider another slice length?!
		recv := make([]byte, 516)
		for {
			n, err := conn.Read(recv)
			if err != nil {
				if nErr, ok := err.(net.Error); ok && nErr.Timeout() {
					continue Tx
				}
				return err
			}
			opcode, remaining, ok := parsePacket(recv[:n])
			if !ok {
				continue
			}
			switch opcode {
			case ACK:
				recvid := binary.BigEndian.Uint16(remaining)
				if recvid == id {
					return nil
				}
				// TODO: what should we do, if the client sends a wrong id?
				log.Printf("RRQ: Received wrong block id (%v) for packet", recvid)
			case ERR:
				// TODO: client aborted the session
			}
		}
	}
	return fmt.Errorf("timed out waiting for ack")
}

func (s *session) sendError(conn net.Conn, errCode uint16, errMsg string) error {
	p := errorPacket(errCode, errMsg)
	_, err := conn.Write(p)
	if err != nil {
		return err
	}
	return nil
}

func fileExists(fname string) bool {
	if _, err := os.Stat(fname); os.IsNotExist(err) {
		return false
	}
	return true
}

func Debug(msg string, v ...interface{}) {
	fmt.Printf("DEBUG: %v\nValues: %v\n", msg, v)
}
