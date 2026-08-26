package main

import (
	"encoding/json"
	"fmt"
	"net"
	"time"

	"golang.org/x/net/ipv4"

	"termchat/shared"
)

// lanInterface pairs an interface with the IPv4 address used to source its
// beacons.
type lanInterface struct {
	iface net.Interface
	ip    net.IP
}

// usableIPv4 returns the interface's first routable IPv4 address, or false
// when the flags or addresses make it unusable for discovery.
func usableIPv4(flags net.Flags, addrs []net.Addr) (net.IP, bool) {
	if flags&net.FlagUp == 0 ||
		flags&net.FlagMulticast == 0 ||
		flags&net.FlagRunning == 0 ||
		flags&net.FlagLoopback != 0 {
		return nil, false
	}

	for _, addr := range addrs {
		ipnet, ok := addr.(*net.IPNet)
		if !ok {
			continue
		}

		ipv4 := ipnet.IP.To4()
		if ipv4 == nil || ipv4.IsLoopback() || ipv4.IsLinkLocalUnicast() {
			continue
		}

		return ipv4, true
	}

	return nil, false
}

// lanInterfaces lists every interface that can carry discovery traffic.
func lanInterfaces() []lanInterface {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}

	var out []lanInterface

	for _, iface := range ifaces {
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		ip, ok := usableIPv4(iface.Flags, addrs)
		if !ok {
			continue
		}

		out = append(out, lanInterface{iface: iface, ip: ip})
	}

	return out
}

// preferredLAN picks the interface owning the default route, else the first.
func preferredLAN(lis []lanInterface, routeIface string) lanInterface {
	for _, li := range lis {
		if li.iface.Name == routeIface {
			return li
		}
	}

	return lis[0]
}

// orderedLAN moves the default-route interface to the front so its beacon is
// the canonical advertisement; the remaining order is preserved.
func orderedLAN(lis []lanInterface, routeIface string) []lanInterface {
	idx := -1

	for i, li := range lis {
		if li.iface.Name == routeIface {
			idx = i

			break
		}
	}

	if idx <= 0 {
		return lis
	}

	out := make([]lanInterface, 0, len(lis))
	out = append(out, lis[idx])
	out = append(out, lis[:idx]...)
	out = append(out, lis[idx+1:]...)

	return out
}

// primaryLANIP is the advertised address for self-hosted status displays;
// it prefers the interface that owns the IPv4 default route.
func primaryLANIP() string {
	lis := lanInterfaces()
	if len(lis) == 0 {
		return "localhost"
	}

	return preferredLAN(lis, defaultRouteIface()).ip.String()
}

// startLANBroadcaster periodically announces this host on every eligible
// interface so that `termchat discover` on the same link can find it.
func startLANBroadcaster(room string, port int, hostNick string) {
	go func() {
		for {
			sendBeacons(lanInterfaces(), room, port, hostNick)
			time.Sleep(1 * time.Second)
		}
	}()
}

// sendBeacons sends one beacon per interface via both multicast and broadcast.
// Failures are skipped; the next tick re-enumerates and retries.
func sendBeacons(lis []lanInterface, room string, port int, hostNick string) {
	lis = orderedLAN(lis, defaultRouteIface())

	group := &net.UDPAddr{
		IP:   net.ParseIP(shared.DiscoveryMulticast),
		Port: shared.DiscoveryPort,
	}

	broadcast := &net.UDPAddr{
		IP:   net.IPv4bcast,
		Port: shared.DiscoveryPort,
	}

	for _, li := range lis {
		beacon := lanBeacon{
			Room: room,
			Port: port,
			Host: hostNick,
			IP:   li.ip.String(),
		}

		payload, err := json.Marshal(beacon)
		if err != nil {
			return
		}

		msg := []byte(fmt.Sprintf("%s|%s", shared.DiscoveryMagic, payload))

		conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: li.ip})
		if err != nil {
			continue
		}

		pc := ipv4.NewPacketConn(conn)

		err = pc.SetMulticastInterface(&li.iface)
		if err == nil {
			err = pc.SetMulticastLoopback(true)
		}
		if err == nil {
			_, err = pc.WriteTo(msg, nil, group)
		}

		conn.Close()

		if err != nil {
			continue
		}

		// Broadcast copy rides the same source-bound socket; some APs drop
		// multicast frames but pass broadcast.
		bconn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: li.ip})
		if err != nil {
			continue
		}

		bconn.WriteToUDP(msg, broadcast)
		bconn.Close()
	}
}
