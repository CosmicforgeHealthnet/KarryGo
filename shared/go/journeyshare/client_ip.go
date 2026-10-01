package journeyshare

import (
	"github.com/gin-gonic/gin"
	"net/netip"
	"strings"
)

// ClientIP trusts forwarding only from explicitly configured gateway networks,
// walking from the socket peer toward the first untrusted hop. Client-supplied
// prefixes of X-Forwarded-For never replace an untrusted connecting address.
func ClientIP(c *gin.Context, gateways []netip.Prefix) string {
	peer, err := netip.ParseAddr(c.RemoteIP())
	if err != nil {
		return c.RemoteIP()
	}
	trusted := func(ip netip.Addr) bool {
		for _, gateway := range gateways {
			if gateway.Contains(ip.Unmap()) {
				return true
			}
		}
		return false
	}
	if !trusted(peer) {
		return peer.String()
	}
	hops := strings.Split(c.GetHeader("X-Forwarded-For"), ",")
	for i := len(hops) - 1; i >= 0; i-- {
		ip, e := netip.ParseAddr(strings.TrimSpace(hops[i]))
		if e != nil {
			return peer.String()
		}
		if !trusted(ip) {
			return ip.Unmap().String()
		}
	}
	return peer.String()
}
