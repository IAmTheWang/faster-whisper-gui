package appserver

import "testing"

func TestRequireLoopbackAddr(t *testing.T) {
	for _, ok := range []string{"127.0.0.1:8080", "localhost:8080", "[::1]:8080", "127.0.0.2:1"} {
		if err := requireLoopbackAddr(ok); err != nil {
			t.Errorf("requireLoopbackAddr(%q) = %v, want nil", ok, err)
		}
	}
	for _, bad := range []string{"0.0.0.0:8080", ":8080", "192.168.1.5:8080", "example.com:80", "nonsense"} {
		if err := requireLoopbackAddr(bad); err == nil {
			t.Errorf("requireLoopbackAddr(%q) = nil, want error", bad)
		}
	}
}
