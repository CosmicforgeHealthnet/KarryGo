package authclients

import (
	"bufio"
	"context"
	"errors"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestNoopEmailClientReportsNotConfigured(t *testing.T) {
	client := NoopEmailClient{}
	if err := client.SendOTP(context.Background(), "user@example.com", "123456"); !errors.Is(err, ErrEmailNotConfigured) {
		t.Fatalf("SendOTP() error = %v, want ErrEmailNotConfigured", err)
	}
	if err := client.SendEmail(context.Background(), "user@example.com", "Subject", "Body", ""); !errors.Is(err, ErrEmailNotConfigured) {
		t.Fatalf("SendEmail() error = %v, want ErrEmailNotConfigured", err)
	}
}

func TestCpanelEmailClientSendEmailUsesGenericSMTPMessage(t *testing.T) {
	server := newFakeSMTPServer(t)
	client := &CpanelEmailClient{
		Host:     "127.0.0.1",
		Port:     server.port,
		TLSMode:  "none",
		User:     "smtp-user",
		Password: "smtp-password",
		From:     "sender@example.com",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := client.SendEmail(ctx, "rider@example.com", "Verification approved", "Plain body", "<p>HTML body</p>"); err != nil {
		t.Fatalf("SendEmail() error = %v", err)
	}

	message := server.message(t)
	for _, want := range []string{
		"From: sender@example.com",
		"To: rider@example.com",
		"Subject: Verification approved",
		"Content-Type: multipart/alternative",
		"Plain body",
		"<p>HTML body</p>",
	} {
		if !strings.Contains(message, want) {
			t.Fatalf("SMTP message missing %q:\n%s", want, message)
		}
	}
	if strings.Contains(message, "smtp-password") {
		t.Fatal("SMTP password leaked into email message")
	}
}

func TestCpanelEmailClientSendOTPMaintainsOTPTemplate(t *testing.T) {
	server := newFakeSMTPServer(t)
	client := &CpanelEmailClient{
		Host:     "127.0.0.1",
		Port:     server.port,
		TLSMode:  "none",
		User:     "smtp-user",
		Password: "smtp-password",
		From:     "sender@example.com",
	}

	if err := client.SendOTP(context.Background(), "rider@example.com", "654321"); err != nil {
		t.Fatalf("SendOTP() error = %v", err)
	}
	message := server.message(t)
	for _, want := range []string{
		"Subject: Your KarryGo Verification Code",
		"Your KarryGo OTP code is: 654321",
		"This code expires in 10 minutes.",
	} {
		if !strings.Contains(message, want) {
			t.Fatalf("SMTP message missing %q:\n%s", want, message)
		}
	}
}

type fakeSMTPServer struct {
	listener net.Listener
	port     int
	messageC chan string
}

func newFakeSMTPServer(t *testing.T) *fakeSMTPServer {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen fake SMTP: %v", err)
	}
	_, portText, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatalf("split fake SMTP addr: %v", err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatalf("parse fake SMTP port: %v", err)
	}
	server := &fakeSMTPServer{listener: listener, port: port, messageC: make(chan string, 1)}
	t.Cleanup(func() { _ = listener.Close() })
	go server.serve()
	return server
}

func (s *fakeSMTPServer) message(t *testing.T) string {
	t.Helper()
	select {
	case message := <-s.messageC:
		return message
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for SMTP message")
		return ""
	}
}

func (s *fakeSMTPServer) serve() {
	conn, err := s.listener.Accept()
	if err != nil {
		return
	}
	defer conn.Close()
	reader := bufio.NewReader(conn)
	writer := bufio.NewWriter(conn)
	writeSMTPLine(writer, "220 fake-smtp.local ESMTP")
	var dataLines []string
	inData := false
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimRight(line, "\r\n")
		if inData {
			if line == "." {
				s.messageC <- strings.Join(dataLines, "\n")
				writeSMTPLine(writer, "250 2.0.0 queued")
				inData = false
				continue
			}
			dataLines = append(dataLines, line)
			continue
		}
		switch {
		case strings.HasPrefix(strings.ToUpper(line), "EHLO") || strings.HasPrefix(strings.ToUpper(line), "HELO"):
			writeSMTPLine(writer, "250-fake-smtp.local")
			writeSMTPLine(writer, "250 AUTH PLAIN")
		case strings.HasPrefix(strings.ToUpper(line), "AUTH"):
			writeSMTPLine(writer, "235 2.7.0 ok")
		case strings.HasPrefix(strings.ToUpper(line), "MAIL FROM"):
			writeSMTPLine(writer, "250 2.1.0 ok")
		case strings.HasPrefix(strings.ToUpper(line), "RCPT TO"):
			writeSMTPLine(writer, "250 2.1.5 ok")
		case strings.EqualFold(line, "DATA"):
			inData = true
			writeSMTPLine(writer, "354 go ahead")
		case strings.EqualFold(line, "QUIT"):
			writeSMTPLine(writer, "221 bye")
			return
		default:
			writeSMTPLine(writer, "250 ok")
		}
	}
}

func writeSMTPLine(writer *bufio.Writer, line string) {
	_, _ = writer.WriteString(line + "\r\n")
	_ = writer.Flush()
}
