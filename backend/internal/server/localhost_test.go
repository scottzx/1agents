package server

import (
	"net/http"
	"testing"
)

func TestIsLocalhost(t *testing.T) {
	cases := []struct {
		remote string
		want   bool
	}{
		{"127.0.0.1:54321", true},
		{"[::1]:54321", true},
		{"192.168.5.200:54321", true},
		{"10.0.0.8:80", true},
		{"172.16.1.1:9", true},
		{"169.254.1.1:9", true},
		{"8.8.8.8:443", false},
		{"1.2.3.4:8080", false},
	}
	for _, tc := range cases {
		r := &http.Request{RemoteAddr: tc.remote}
		if got := isLocalhost(r); got != tc.want {
			t.Errorf("isLocalhost(%q) = %v, want %v", tc.remote, got, tc.want)
		}
	}
}
