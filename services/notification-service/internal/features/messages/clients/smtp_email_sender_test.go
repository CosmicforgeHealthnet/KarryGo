package messageclients

import (
	"bufio"
	"context"
	"cosmicforge/logistics/services/notification-service/internal/config"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"
)

func testSMTPConfig(port int) config.SMTPConfig {
	return config.SMTPConfig{Host: "127.0.0.1", Port: port, Username: "smtp-user", Password: "smtp-password", From: "sender@example.com", TLSMode: "none", Timeout: time.Second}
}
func TestSMTPSendsEmailAndSanitizesHeaders(t *testing.T) {
	server := newFakeSMTPServer(t)
	sender := NewSMTPEmailSender(testSMTPConfig(server.port))
	result, err := sender.SendEmail(context.Background(), EmailMessage{To: "rider@example.com", Subject: "Code\r\nBcc: injected@example.com", Body: "Your verification code is 123456"})
	if err != nil || result.Provider != "smtp" {
		t.Fatalf("send: %+v %v", result, err)
	}
	message := server.message(t)
	if !strings.Contains(message, "Your verification code is 123456") || strings.Contains(message, "\nBcc:") || strings.Contains(message, "smtp-password") {
		t.Fatal("unsafe or incomplete email")
	}
}
func TestSMTPRequiresSTARTTLS(t *testing.T) {
	server := newFakeSMTPServer(t)
	cfg := testSMTPConfig(server.port)
	cfg.TLSMode = "starttls"
	_, err := NewSMTPEmailSender(cfg).SendEmail(context.Background(), EmailMessage{To: "rider@example.com"})
	if err == nil || !strings.Contains(err.Error(), "STARTTLS") {
		t.Fatalf("expected TLS refusal: %v", err)
	}
}
func TestSMTPTimeoutBoundsServerGreeting(t *testing.T) {
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
		var b [1]byte
		_, _ = conn.Read(b[:])
	}()
	cfg := testSMTPConfig(listener.Addr().(*net.TCPAddr).Port)
	cfg.Timeout = 80 * time.Millisecond
	start := time.Now()
	result, err := NewSMTPEmailSender(cfg).SendEmail(context.Background(), EmailMessage{To: "rider@example.com"})
	if err == nil || !result.Retryable || time.Since(start) > time.Second {
		t.Fatalf("unbounded send: %+v %v", result, err)
	}
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("connection leaked")
	}
}
func TestSMTPRejectsMissingConfigWithoutSuccess(t *testing.T) {
	result, err := NewSMTPEmailSender(config.SMTPConfig{}).SendEmail(context.Background(), EmailMessage{To: "rider@example.com"})
	if err == nil || result.Retryable || result.ProviderMessageID != nil {
		t.Fatal("unconfigured sender reported delivery")
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

func TestSMTPReadyRefusesPlaintextToRemoteHost(t *testing.T) {
	base := config.SMTPConfig{Host: "smtp.resend.com", Port: 465, Username: "resend", Password: "re_not_a_real_key", From: "Karryam <no-reply@karryam.com>", Timeout: time.Second}
	for _, mode := range []string{"", "tls", "starttls", "TLS"} {
		cfg := base
		cfg.TLSMode = mode
		if err := NewSMTPEmailSender(cfg).Ready(); err != nil {
			t.Fatalf("mode %q: unexpected error %v", mode, err)
		}
	}
	cfg := base
	cfg.TLSMode = "none"
	if err := NewSMTPEmailSender(cfg).Ready(); err == nil {
		t.Fatal("plain SMTP to a remote provider must be refused")
	}
	for _, host := range []string{"127.0.0.1", "localhost", "::1", "[::1]"} {
		local := base
		local.Host = host
		local.TLSMode = "none"
		if err := NewSMTPEmailSender(local).Ready(); err != nil {
			t.Fatalf("loopback %q: %v", host, err)
		}
	}
}
