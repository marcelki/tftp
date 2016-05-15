package netascii

import (
	"bufio"
	"bytes"
	"io"
)

func WriteTo(b []byte, bw *bufio.Writer) (n int, err error) {
	b = bytes.Replace(b, []byte{'\r', 0}, []byte{'\r'}, -1)
	b = bytes.Replace(b, []byte{'\r', '\n'}, []byte{'\n'}, -1)
	n, err = bw.Write(b)
	if err != nil {
		return 0, err
	}
	return n, nil
}

func Convert(b []byte) []byte {
	b = bytes.Replace(b, []byte{'\r', 0}, []byte{'\r'}, -1)
	b = bytes.Replace(b, []byte{'\r', '\n'}, []byte{'\n'}, -1)
	return b
}

func ReadFull(r io.Reader, buf []byte) (n int, err error) {
	if len(buf) == 0 {
		return 0, io.ErrShortBuffer
	}
	n, err = r.Read(buf)
	if err != nil {
		return 0, err
	}
	buf = bytes.Replace(buf, []byte{'\r', 0}, []byte{'\r'}, -1)
	buf = bytes.Replace(buf, []byte{'\r', '\n'}, []byte{'\n'}, -1)

	return n, nil
}
