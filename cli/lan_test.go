package main

import (
	"net"
	"testing"
	"time"

	"termchat/shared"
)

func ipnet4(s string) net.Addr {
	return &net.IPNet{IP: net.ParseIP(s), Mask: net.CIDRMask(24, 32)}
}

func ipnet6(s string) net.Addr {
	return &net.IPNet{IP: net.ParseIP(s), Mask: net.CIDRMask(64, 128)}
}

func TestUsableIPv4(t *testing.T) {
	up := net.FlagUp | net.FlagMulticast | net.FlagRunning

	cases := []struct {
		name  string
		flags net.Flags
		addrs []net.Addr
		want  string
	}{
		{"routable", up, []net.Addr{ipnet4("192.168.1.10")}, "192.168.1.10"},
		{"loopback interface", up | net.FlagLoopback, []net.Addr{ipnet4("127.0.0.1")}, ""},
		{"down interface", 0, []net.Addr{ipnet4("192.168.1.10")}, ""},
		{"no multicast flag", net.FlagUp | net.FlagRunning, []net.Addr{ipnet4("192.168.1.10")}, ""},
		{"no carrier", net.FlagUp | net.FlagMulticast, []net.Addr{ipnet4("172.17.0.1")}, ""},
		{"ipv6 only", up, []net.Addr{ipnet6("fd00::1")}, ""},
		{"link-local only", up, []net.Addr{ipnet4("169.254.9.9")}, ""},
		{
			"first usable wins",
			up,
			[]net.Addr{ipnet4("169.254.9.9"), ipnet6("fd00::1"), ipnet4("10.0.0.5")},
			"10.0.0.5",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ip, ok := usableIPv4(tc.flags, tc.addrs)

			if tc.want == "" {
				if ok {
					t.Fatalf("got %v, want none", ip)
				}

				return
			}

			if !ok || ip.String() != tc.want {
				t.Fatalf("got (%v, %v), want %s", ip, ok, tc.want)
			}
		})
	}
}

func TestParseBeacon(t *testing.T) {
	src := &net.UDPAddr{IP: net.ParseIP("10.0.0.1")}

	payload := shared.DiscoveryMagic + `|{"room":"FROG","port":8080,"host":"alice","ip":"192.168.1.10"}`
	beacon, ok := parseBeacon([]byte(payload), src)
	if !ok {
		t.Fatal("valid beacon rejected")
	}

	if beacon.Room != "FROG" || beacon.Port != 8080 || beacon.Host != "alice" || beacon.IP != "192.168.1.10" {
		t.Fatalf("beacon = %+v", beacon)
	}

	fallback, ok := parseBeacon(
		[]byte(shared.DiscoveryMagic+`|{"room":"FROG","port":9000}`),
		src,
	)
	if !ok || fallback.IP != "10.0.0.1" {
		t.Fatalf("source fallback = (%+v, %v)", fallback, ok)
	}

	if _, ok := parseBeacon([]byte(`NOT_A_BEACON|{}`), src); ok {
		t.Error("wrong magic accepted")
	}

	if _, ok := parseBeacon([]byte(shared.DiscoveryMagic+`|not json`), src); ok {
		t.Error("malformed payload accepted")
	}

	if _, ok := parseBeacon([]byte(shared.DiscoveryMagic+`|{"room":"FROG"}`), nil); ok {
		t.Error("beacon without address accepted")
	}
}

func TestSendBeaconsEmptyInterfaces(t *testing.T) {
	sendBeacons(nil, "FROG", 8080, "alice")
}

func TestSendBeaconsSmoke(t *testing.T) {
	lis := []lanInterface{{iface: net.Interface{Name: "lo"}, ip: net.ParseIP("127.0.0.1").To4()}}

	sendBeacons(lis, "FROG", 8080, "alice")
}

func TestPreferredLAN(t *testing.T) {
	lis := []lanInterface{
		{iface: net.Interface{Name: "docker0"}, ip: net.ParseIP("172.17.0.1").To4()},
		{iface: net.Interface{Name: "wlp0s20f3"}, ip: net.ParseIP("10.28.26.116").To4()},
	}

	if got := preferredLAN(lis, "wlp0s20f3"); got.iface.Name != "wlp0s20f3" {
		t.Errorf("preferred = %q, want wlp0s20f3", got.iface.Name)
	}

	if got := preferredLAN(lis, ""); got.iface.Name != "docker0" {
		t.Errorf("fallback = %q, want docker0 (first)", got.iface.Name)
	}
}

func TestOrderedLAN(t *testing.T) {
	lis := []lanInterface{
		{iface: net.Interface{Name: "docker0"}},
		{iface: net.Interface{Name: "br-1"}},
		{iface: net.Interface{Name: "wlp0s20f3"}},
		{iface: net.Interface{Name: "incusbr0"}},
	}

	got := orderedLAN(lis, "wlp0s20f3")

	want := []string{"wlp0s20f3", "docker0", "br-1", "incusbr0"}

	for i, name := range want {
		if got[i].iface.Name != name {
			t.Fatalf("order[%d] = %q, want %q (got %v)", i, got[i].iface.Name, name, got)
		}
	}

	if ordered := orderedLAN(lis, ""); ordered[0].iface.Name != "docker0" {
		t.Errorf("no-route ordering changed the head: %q", ordered[0].iface.Name)
	}
}

// TestBeaconRoundTrip verifies a real host hears its own beacon, the
// same-host path that broke on multi-homed machines before per-interface
// sockets.
func TestBeaconRoundTrip(t *testing.T) {
	lis := lanInterfaces()
	if len(lis) == 0 {
		t.Skip("no discovery-capable interface")
	}

	found := make(chan []lanBeacon, 1)

	go func() {
		found <- listenForBeacons(3 * time.Second)
	}()

	time.Sleep(100 * time.Millisecond)
	sendBeacons(lis, "FROG", 8080, "alice")

	beacons := <-found

	for _, b := range beacons {
		if b.Room == "FROG" && b.Port == 8080 && b.Host == "alice" {
			return
		}
	}

	t.Fatalf("own beacon not heard, got %+v", beacons)
}
