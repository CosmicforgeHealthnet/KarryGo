package partneraccount

import (
	"bufio"
	"context"
	"io"
	"net"
	"strconv"
	"strings"
	"testing"

	"cosmicforge/logistics/services/partner-web-service/internal/config"
)

func TestAcceptedOTPDoesNotFailWhenServerDisconnectsOnQuit(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		reader := bufio.NewReader(conn)
		_, _ = io.WriteString(conn, "220 local test\r\n")
		data := false
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				return
			}
			line = strings.TrimSpace(line)
			if data {
				if line == "." {
					_, _ = io.WriteString(conn, "250 accepted\r\n")
					data = false
				}
				continue
			}
			switch {
			case strings.HasPrefix(line, "EHLO"):
				_, _ = io.WriteString(conn, "250-local\r\n250 AUTH PLAIN\r\n")
			case strings.HasPrefix(line, "AUTH"):
				_, _ = io.WriteString(conn, "235 authenticated\r\n")
			case line == "DATA":
				data = true
				_, _ = io.WriteString(conn, "354 send\r\n")
			case line == "QUIT":
				return // DATA was accepted, but no QUIT response arrives.
			default:
				_, _ = io.WriteString(conn, "250 ok\r\n")
			}
		}
	}()
	port, _ := strconv.Atoi(strings.Split(listener.Addr().String(), ":")[1])
	cfg := config.PartnerSMTPConfig{Host: "127.0.0.1", Port: port, Username: "test", Password: "test", From: "sender@example.com"}
	// Exercise the wire helper with a loopback relay; production sender still
	// requires TLS. No external recipients or provider credentials are involved.
	if err := sendSMTPMessage(context.Background(), cfg, "test@example.com", []byte("Subject: test\r\n\r\nbody\r\n")); err != nil {
		t.Fatalf("accepted OTP treated as failed: %v", err)
	}
	<-finished
}
