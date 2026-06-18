package messageclients

import (
	"bufio"
	"context"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
)

type WebSocketHub struct {
	mu      sync.RWMutex
	clients map[string]map[*webSocketClient]struct{}
}

func NewWebSocketHub() *WebSocketHub {
	return &WebSocketHub{clients: map[string]map[*webSocketClient]struct{}{}}
}

func (h *WebSocketHub) ServeHTTP(w http.ResponseWriter, r *http.Request, recipientType string, recipientID string) error {
	if !isWebSocketRequest(r) {
		return fmt.Errorf("request is not a websocket upgrade")
	}
	hijacker, ok := w.(http.Hijacker)
	if !ok {
		return fmt.Errorf("websocket upgrade is not supported")
	}
	conn, readerWriter, err := hijacker.Hijack()
	if err != nil {
		return err
	}

	key := r.Header.Get("Sec-WebSocket-Key")
	accept := webSocketAccept(key)
	response := "HTTP/1.1 101 Switching Protocols\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Accept: " + accept + "\r\n\r\n"
	if _, err := readerWriter.WriteString(response); err != nil {
		_ = conn.Close()
		return err
	}
	if err := readerWriter.Flush(); err != nil {
		_ = conn.Close()
		return err
	}

	client := &webSocketClient{conn: conn}
	recipientKey := realtimeRecipientKey(recipientType, recipientID)
	h.add(recipientKey, client)
	go h.readUntilClose(recipientKey, client, readerWriter.Reader)
	return nil
}

func (h *WebSocketHub) SendRealtime(ctx context.Context, message RealtimeMessage) error {
	payload, err := json.Marshal(map[string]interface{}{
		"event_type": message.EventType,
		"title":      message.Title,
		"body":       message.Body,
		"data":       message.Data,
	})
	if err != nil {
		return err
	}

	recipientKey := realtimeRecipientKey(message.RecipientType, message.RecipientID)
	h.mu.RLock()
	clients := h.clients[recipientKey]
	snapshot := make([]*webSocketClient, 0, len(clients))
	for client := range clients {
		snapshot = append(snapshot, client)
	}
	h.mu.RUnlock()

	for _, client := range snapshot {
		if err := client.writeText(payload); err != nil {
			h.remove(recipientKey, client)
		}
	}
	return nil
}

func (h *WebSocketHub) add(recipientKey string, client *webSocketClient) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.clients[recipientKey] == nil {
		h.clients[recipientKey] = map[*webSocketClient]struct{}{}
	}
	h.clients[recipientKey][client] = struct{}{}
}

func (h *WebSocketHub) remove(recipientKey string, client *webSocketClient) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.clients[recipientKey] != nil {
		delete(h.clients[recipientKey], client)
		if len(h.clients[recipientKey]) == 0 {
			delete(h.clients, recipientKey)
		}
	}
	_ = client.conn.Close()
}

func (h *WebSocketHub) readUntilClose(recipientKey string, client *webSocketClient, reader *bufio.Reader) {
	buffer := make([]byte, 512)
	for {
		if _, err := reader.Read(buffer); err != nil {
			h.remove(recipientKey, client)
			return
		}
	}
}

type webSocketClient struct {
	conn net.Conn
	mu   sync.Mutex
}

func (c *webSocketClient) writeText(payload []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	frame := encodeTextFrame(payload)
	_, err := c.conn.Write(frame)
	return err
}

func isWebSocketRequest(r *http.Request) bool {
	return strings.EqualFold(r.Header.Get("Upgrade"), "websocket") &&
		strings.Contains(strings.ToLower(r.Header.Get("Connection")), "upgrade") &&
		r.Header.Get("Sec-WebSocket-Key") != ""
}

func webSocketAccept(key string) string {
	hash := sha1.Sum([]byte(key + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"))
	return base64.StdEncoding.EncodeToString(hash[:])
}

func encodeTextFrame(payload []byte) []byte {
	header := []byte{0x81}
	switch {
	case len(payload) < 126:
		header = append(header, byte(len(payload)))
	case len(payload) <= 65535:
		header = append(header, 126, 0, 0)
		binary.BigEndian.PutUint16(header[len(header)-2:], uint16(len(payload)))
	default:
		header = append(header, 127, 0, 0, 0, 0, 0, 0, 0, 0)
		binary.BigEndian.PutUint64(header[len(header)-8:], uint64(len(payload)))
	}
	return append(header, payload...)
}

func realtimeRecipientKey(recipientType string, recipientID string) string {
	return recipientType + ":" + recipientID
}
