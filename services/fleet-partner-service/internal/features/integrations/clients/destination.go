// Package clients holds the outbound webhook transport: destination (SSRF)
// validation and the hardened HTTP client used for delivery.
package clients

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// ErrUnsafeDestination means a webhook destination is not permitted.
var ErrUnsafeDestination = errors.New("integrations: webhook destination is not permitted")

type IPResolver interface {
	LookupIPAddr(context.Context, string) ([]net.IPAddr, error)
}

type DestinationValidator struct {
	Resolver          IPResolver
	Production        bool
	AllowHTTP         bool
	AllowedPrivate    []*net.IPNet
	AllowTestLoopback bool
}

func NewDestinationValidator(production, allowHTTP bool, privateCIDRs []string) (*DestinationValidator, error) {
	v := &DestinationValidator{Resolver: net.DefaultResolver, Production: production, AllowHTTP: allowHTTP}
	for _, raw := range privateCIDRs {
		_, network, err := net.ParseCIDR(strings.TrimSpace(raw))
		if err != nil {
			return nil, fmt.Errorf("integrations: invalid private destination CIDR %q", raw)
		}
		v.AllowedPrivate = append(v.AllowedPrivate, network)
	}
	return v, nil
}

func (v *DestinationValidator) Validate(ctx context.Context, raw string) (*url.URL, []net.IP, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" || u.User != nil || u.Fragment != "" {
		return nil, nil, ErrUnsafeDestination
	}
	if u.Scheme != "https" && !(u.Scheme == "http" && v.AllowHTTP && !v.Production) {
		return nil, nil, ErrUnsafeDestination
	}
	if _, err = parsePort(u); err != nil {
		return nil, nil, ErrUnsafeDestination
	}
	host := u.Hostname()
	var ips []net.IP
	if literal := net.ParseIP(host); literal != nil {
		ips = []net.IP{literal}
	} else {
		resolver := v.Resolver
		if resolver == nil {
			resolver = net.DefaultResolver
		}
		resolved, lookupErr := resolver.LookupIPAddr(ctx, host)
		if lookupErr != nil || len(resolved) == 0 {
			return nil, nil, ErrUnsafeDestination
		}
		for _, item := range resolved {
			ips = append(ips, item.IP)
		}
	}
	if err = v.ValidateIPs(ips); err != nil {
		return nil, nil, err
	}
	return u, ips, nil
}

func parsePort(u *url.URL) (string, error) {
	port := u.Port()
	if port == "" {
		if u.Scheme == "https" {
			return "443", nil
		}
		return "80", nil
	}
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 {
		return "", ErrUnsafeDestination
	}
	return port, nil
}

// ValidateIPs refuses unspecified, multicast, link-local, loopback (outside
// tests), metadata and non-allowlisted private addresses.
func (v *DestinationValidator) ValidateIPs(ips []net.IP) error {
	for _, ip := range ips {
		if ip == nil || ip.IsUnspecified() || ip.IsMulticast() || ip.IsLinkLocalMulticast() || ip.IsLinkLocalUnicast() {
			return ErrUnsafeDestination
		}
		if ip.IsLoopback() {
			if !v.AllowTestLoopback || v.Production {
				return ErrUnsafeDestination
			}
			continue
		}
		if isMetadataIP(ip) {
			return ErrUnsafeDestination
		}
		if ip.IsPrivate() && !v.privateAllowed(ip) {
			return ErrUnsafeDestination
		}
	}
	return nil
}

func (v *DestinationValidator) privateAllowed(ip net.IP) bool {
	for _, network := range v.AllowedPrivate {
		if network.Contains(ip) {
			return true
		}
	}
	return false
}

func isMetadataIP(ip net.IP) bool {
	return ip.Equal(net.ParseIP("169.254.169.254")) || ip.Equal(net.ParseIP("fd00:ec2::254"))
}

// SecureHTTPClient dials only destinations the validator accepts, at dial
// time and on every redirect, so DNS rebinding cannot reach a private host.
func SecureHTTPClient(validator *DestinationValidator, timeout time.Duration) *http.Client {
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	dialer := &net.Dialer{Timeout: timeout, KeepAlive: 30 * time.Second}
	transport := &http.Transport{
		Proxy: nil,
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(address)
			if err != nil {
				return nil, ErrUnsafeDestination
			}
			var ips []net.IP
			if literal := net.ParseIP(host); literal != nil {
				ips = []net.IP{literal}
			} else {
				resolver := validator.Resolver
				if resolver == nil {
					resolver = net.DefaultResolver
				}
				resolved, lookupErr := resolver.LookupIPAddr(ctx, host)
				if lookupErr != nil {
					return nil, ErrUnsafeDestination
				}
				for _, item := range resolved {
					ips = append(ips, item.IP)
				}
			}
			if err = validator.ValidateIPs(ips); err != nil {
				return nil, err
			}
			for _, ip := range ips {
				conn, dialErr := dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
				if dialErr == nil {
					return conn, nil
				}
			}
			return nil, errors.New("integrations: webhook destination connection failed")
		},
		TLSHandshakeTimeout:   timeout,
		ResponseHeaderTimeout: timeout,
		ExpectContinueTimeout: time.Second,
		MaxIdleConns:          50,
		IdleConnTimeout:       30 * time.Second,
	}
	client := &http.Client{Transport: transport, Timeout: timeout}
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 3 {
			return errors.New("integrations: webhook redirect limit exceeded")
		}
		_, _, err := validator.Validate(req.Context(), req.URL.String())
		return err
	}
	return client
}

// ParseRetryAfter reads a Retry-After header (seconds or HTTP date), capped
// at 24 hours; anything else is zero.
func ParseRetryAfter(raw string, now time.Time) time.Duration {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0
	}
	if seconds, err := strconv.Atoi(raw); err == nil && seconds > 0 {
		delay := time.Duration(seconds) * time.Second
		if delay > 24*time.Hour {
			return 24 * time.Hour
		}
		return delay
	}
	if when, err := http.ParseTime(raw); err == nil && when.After(now) {
		delay := when.Sub(now)
		if delay > 24*time.Hour {
			return 24 * time.Hour
		}
		return delay
	}
	return 0
}
