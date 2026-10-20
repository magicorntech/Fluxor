package httpapi

import (
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"strings"
)

// ParseTrustedProxies turns CIDR blocks or single addresses ("10.0.0.0/8", "192.168.1.5") into prefixes.
func ParseTrustedProxies(values []string) ([]netip.Prefix, error) {
	out := make([]netip.Prefix, 0, len(values))
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		if p, err := netip.ParsePrefix(v); err == nil {
			out = append(out, p.Masked())
			continue
		}
		a, err := netip.ParseAddr(v)
		if err != nil {
			return nil, fmt.Errorf("%q is neither a CIDR block nor an IP address", v)
		}
		out = append(out, netip.PrefixFrom(a, a.BitLen()))
	}
	return out, nil
}

func isTrusted(a netip.Addr, trusted []netip.Prefix) bool {
	for _, p := range trusted {
		if p.Contains(a.Unmap()) {
			return true
		}
	}
	return false
}

// clientIP returns the address the request really came from. X-Forwarded-For is believed only when
// the direct peer is one of the trusted proxies, because anyone else can put anything in that header,
// which would let a client choose the address that rate limits and audit entries are keyed on. It
// walks the header from the right and returns the first address that is not a trusted proxy.
func clientIP(r *http.Request, trusted []netip.Prefix) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	peer, err := netip.ParseAddr(host)
	if err != nil || len(trusted) == 0 || !isTrusted(peer, trusted) {
		return host
	}
	hops := strings.Split(strings.Join(r.Header.Values("X-Forwarded-For"), ","), ",")
	for i := len(hops) - 1; i >= 0; i-- {
		a, err := netip.ParseAddr(strings.TrimSpace(hops[i]))
		if err != nil {
			return host // a garbled header is not believed at all
		}
		if !isTrusted(a, trusted) {
			return a.Unmap().String()
		}
	}
	return host
}
