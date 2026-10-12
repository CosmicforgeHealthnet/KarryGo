package clamav

import (
	"bytes"
	"context"
	media "cosmicforge/logistics/services/media-file-service/internal/features/mediaobjects/usecases"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"strings"
	"time"
)

type Scanner struct {
	network, address string
	timeout          time.Duration
	maxBytes         int64
}

func New(network, address string, timeout time.Duration, maxBytes int64) (*Scanner, error) {
	if network != "tcp" && network != "unix" {
		return nil, fmt.Errorf("unsupported scanner network")
	}
	if network == "tcp" {
		if _, _, err := net.SplitHostPort(address); err != nil {
			return nil, fmt.Errorf("invalid scanner address")
		}
	} else if strings.TrimSpace(address) == "" {
		return nil, fmt.Errorf("scanner address required")
	}
	if timeout <= 0 || maxBytes <= 0 {
		return nil, fmt.Errorf("scanner limits required")
	}
	return &Scanner{network, address, timeout, maxBytes}, nil
}
func (s *Scanner) Name() string { return "clamav" }
func (s *Scanner) Scan(ctx context.Context, _ string, content []byte) (media.ScanResult, error) {
	if len(content) == 0 || int64(len(content)) > s.maxBytes {
		return media.ScanResult{}, fmt.Errorf("unsupported scan size")
	}
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	conn, err := (&net.Dialer{}).DialContext(ctx, s.network, s.address)
	if err != nil {
		return media.ScanResult{}, fmt.Errorf("scanner unavailable")
	}
	defer conn.Close()
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()
	deadline, _ := ctx.Deadline()
	if err = conn.SetDeadline(deadline); err != nil {
		return media.ScanResult{}, err
	}
	write := func(b []byte) error { _, e := io.Copy(conn, bytes.NewReader(b)); return e }
	if err = write([]byte("zINSTREAM\x00")); err != nil {
		return media.ScanResult{}, fmt.Errorf("scanner write failed")
	}
	var length [4]byte
	for offset := 0; offset < len(content); {
		end := offset + 32*1024
		if end > len(content) {
			end = len(content)
		}
		binary.BigEndian.PutUint32(length[:], uint32(end-offset))
		if err = write(length[:]); err == nil {
			err = write(content[offset:end])
		}
		if err != nil {
			return media.ScanResult{}, fmt.Errorf("scanner write failed")
		}
		offset = end
	}
	if err = write([]byte{0, 0, 0, 0}); err != nil {
		return media.ScanResult{}, fmt.Errorf("scanner write failed")
	}
	reply, err := io.ReadAll(io.LimitReader(conn, 4097))
	if err != nil || len(reply) > 4096 || ctx.Err() != nil {
		return media.ScanResult{}, fmt.Errorf("scanner did not return a complete verdict")
	}
	if string(reply) == "stream: OK\x00" {
		return media.ScanResult{Safe: true}, nil
	}
	if bytes.HasPrefix(reply, []byte("stream: ")) && bytes.HasSuffix(reply, []byte(" FOUND\x00")) && bytes.Count(reply, []byte{0}) == 1 {
		return media.ScanResult{Reason: "CLAMAV_THREAT_DETECTED"}, nil
	}
	return media.ScanResult{}, fmt.Errorf("scanner returned an unverified verdict")
}

var _ media.Scanner = (*Scanner)(nil)
