package router

import (
	"net"
	"testing"

	"github.com/valyala/fasthttp"
)

func TestGetClientIPOnlyTrustsLoopbackProxy(t *testing.T) {
	tests := []struct {
		name       string
		remoteAddr net.Addr
		expectedIP string
	}{
		{
			name:       "loopback nginx may provide forwarded identity",
			remoteAddr: &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 8080},
			expectedIP: "198.51.100.10",
		},
		{
			name:       "private peer cannot provide forwarded identity",
			remoteAddr: &net.TCPAddr{IP: net.ParseIP("192.168.1.20"), Port: 4321},
			expectedIP: "192.168.1.20",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := &fasthttp.RequestCtx{}
			ctx.SetRemoteAddr(tt.remoteAddr)
			ctx.Request.Header.Set("X-Real-IP", "198.51.100.10")

			if got := getClientIP(ctx); got != tt.expectedIP {
				t.Fatalf("getClientIP() = %q, want %q", got, tt.expectedIP)
			}
		})
	}
}

func TestIsTrustedProxy(t *testing.T) {
	if !isTrustedProxy(net.ParseIP("127.0.0.1")) {
		t.Fatal("loopback should be trusted as the nginx peer")
	}
	if isTrustedProxy(net.ParseIP("10.0.0.2")) {
		t.Fatal("private peers must not be trusted as forwarding proxies")
	}
	if isTrustedProxy(net.ParseIP("203.0.113.10")) {
		t.Fatal("public peers must not be trusted as forwarding proxies")
	}
}
