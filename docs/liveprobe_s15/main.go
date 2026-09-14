// Command liveprobe_s15 certifies the Sections 15-18 surfaces against REAL
// running services rather than in-process handlers.
//
// It builds and boots the actual driver-hauling-service and
// driver-dispatch-delivery-service binaries against the certification
// PostgreSQL and Redis, then drives their HTTP surfaces exactly as Vendor
// Service, Fleet and an operator console would: HMAC-v2 signed internal
// calls, bearer-token provider calls, and unsigned calls that must be
// refused.
//
// Everything it reports as PASS actually executed against a process listening
// on a socket. Anything it could not run is reported as SKIP with the reason,
// never as a pass.
package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"cosmicforge/logistics/shared/go/serviceauth"
)

type probe struct {
	passed, failed, skipped int
	out                     *os.File
}

func (p *probe) pass(name, detail string) {
	p.passed++
	fmt.Fprintf(p.out, "PASS %-62s %s\n", name, detail)
}

func (p *probe) fail(name, detail string) {
	p.failed++
	fmt.Fprintf(p.out, "FAIL %-62s %s\n", name, detail)
}

func (p *probe) skip(name, reason string) {
	p.skipped++
	fmt.Fprintf(p.out, "SKIP %-62s %s\n", name, reason)
}

func (p *probe) expect(name string, want int, got int, extra string) {
	if got == want {
		p.pass(name, fmt.Sprintf("-> %d %s", got, extra))
		return
	}
	p.fail(name, fmt.Sprintf("-> %d, want %d %s", got, want, extra))
}

// service is one booted binary.
type service struct {
	name    string
	baseURL string
	cmd     *exec.Cmd
	logs    *bytes.Buffer
}

func (s *service) stop() {
	if s.cmd != nil && s.cmd.Process != nil {
		_ = s.cmd.Process.Kill()
		_ = s.cmd.Wait()
	}
}

func freePort() (string, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", err
	}
	address := listener.Addr().String()
	return address, listener.Close()
}

func boot(name, moduleDir string, env []string) (*service, error) {
	root, err := filepath.Abs(moduleDir)
	if err != nil {
		return nil, err
	}
	binary := filepath.Join(os.TempDir(), name+"-liveprobe")
	build := exec.Command("go", "build", "-o", binary, "./cmd")
	build.Dir = root
	if output, buildErr := build.CombinedOutput(); buildErr != nil {
		return nil, fmt.Errorf("build %s: %w\n%s", name, buildErr, output)
	}
	address, err := freePort()
	if err != nil {
		return nil, err
	}
	command := exec.Command(binary)
	command.Dir = root
	command.Env = append(append(os.Environ(), "HTTP_ADDR="+address), env...)
	logs := &bytes.Buffer{}
	command.Stdout, command.Stderr = logs, logs
	if err = command.Start(); err != nil {
		return nil, err
	}
	svc := &service{name: name, baseURL: "http://" + address, cmd: command, logs: logs}

	deadline := time.Now().Add(45 * time.Second)
	for {
		response, requestErr := http.Get(svc.baseURL + "/health")
		if requestErr == nil {
			_ = response.Body.Close()
			if response.StatusCode == http.StatusOK {
				return svc, nil
			}
		}
		if time.Now().After(deadline) {
			svc.stop()
			return nil, fmt.Errorf("%s did not become healthy: %v\n%s", name, requestErr, logs.String())
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// call performs one request, optionally HMAC-signed, and returns status+body.
func call(method, url, caller string, secret []byte, body []byte, bearer string) (int, []byte, error) {
	var reader io.Reader
	if len(body) > 0 {
		reader = bytes.NewReader(body)
	}
	request, err := http.NewRequestWithContext(context.Background(), method, url, reader)
	if err != nil {
		return 0, nil, err
	}
	request.Header.Set("Accept", "application/json")
	if len(body) > 0 {
		request.Header.Set("Content-Type", "application/json")
	}
	if bearer != "" {
		request.Header.Set("Authorization", "Bearer "+bearer)
	}
	if caller != "" {
		if err = serviceauth.SignRequest(request, caller, secret, body, time.Now()); err != nil {
			return 0, nil, err
		}
	}
	response, err := (&http.Client{Timeout: 20 * time.Second}).Do(request)
	if err != nil {
		return 0, nil, err
	}
	defer response.Body.Close()
	payload, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	return response.StatusCode, payload, err
}

func main() {
	p := &probe{out: os.Stdout}
	fmt.Fprintln(p.out, "=== Sections 15-18 live certification against running services ===")
	fmt.Fprintln(p.out)

	haulingDSN := os.Getenv("HAULING_LIVE_DATABASE_URL")
	dispatchDSN := os.Getenv("DISPATCH_LIVE_DATABASE_URL")
	redisAddr := os.Getenv("LIVE_REDIS_ADDR")
	if haulingDSN == "" || dispatchDSN == "" || redisAddr == "" {
		fmt.Fprintln(p.out, "HAULING_LIVE_DATABASE_URL, DISPATCH_LIVE_DATABASE_URL and LIVE_REDIS_ADDR are required")
		os.Exit(2)
	}

	certifyHauling(p, haulingDSN, redisAddr)
	fmt.Fprintln(p.out)
	certifyDispatch(p, dispatchDSN, redisAddr)

	fmt.Fprintln(p.out)
	fmt.Fprintf(p.out, "passed=%d failed=%d skipped=%d\n", p.passed, p.failed, p.skipped)
	if p.failed > 0 {
		os.Exit(1)
	}
}
