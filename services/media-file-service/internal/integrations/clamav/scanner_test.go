package clamav

import (
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"net"
	"testing"
	"time"
)

func TestScannerStreamAndVerdicts(t *testing.T) {
	for _, test := range []struct {
		name, reply string
		safe, fail  bool
	}{{"clean", "stream: OK\x00", true, false}, {"infected", "stream: Test-Signature FOUND\x00", false, false}, {"error", "stream: size limit exceeded ERROR\x00", false, true}, {"truncated", "stream: OK", false, true}, {"extra verdict", "stream: OK\x00stream: FOUND\x00", false, true}} {
		t.Run(test.name, func(t *testing.T) {
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer listener.Close()
			finished := make(chan error, 1)
			content := bytes.Repeat([]byte("photo"), 10000)
			go func() {
				conn, e := listener.Accept()
				if e != nil {
					finished <- e
					return
				}
				defer conn.Close()
				_ = conn.SetDeadline(time.Now().Add(time.Second))
				header := make([]byte, 10)
				_, e = io.ReadFull(conn, header)
				if e != nil || string(header) != "zINSTREAM\x00" {
					finished <- io.ErrUnexpectedEOF
					return
				}
				var scanned []byte
				for {
					var size uint32
					if e = binary.Read(conn, binary.BigEndian, &size); e != nil {
						finished <- e
						return
					}
					if size == 0 {
						break
					}
					if size > 32768 {
						finished <- io.ErrShortBuffer
						return
					}
					chunk := make([]byte, size)
					if _, e = io.ReadFull(conn, chunk); e != nil {
						finished <- e
						return
					}
					scanned = append(scanned, chunk...)
				}
				if !bytes.Equal(scanned, content) {
					finished <- io.ErrUnexpectedEOF
					return
				}
				_, e = io.WriteString(conn, test.reply)
				finished <- e
			}()
			scanner, err := New("tcp", listener.Addr().String(), time.Second, 100000)
			if err != nil {
				t.Fatal(err)
			}
			result, err := scanner.Scan(context.Background(), "media-a", content)
			if (err != nil) != test.fail || result.Safe != test.safe {
				t.Fatalf("unexpected verdict: %+v %v", result, err)
			}
			if err = <-finished; err != nil {
				t.Fatal(err)
			}
		})
	}
}
