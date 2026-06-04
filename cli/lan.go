package main

import (
	"encoding/json"
	"fmt"
	"net"
	"time"

	"termchat/shared"
)

func GetLocalIP() string {
	interfaces, err := net.Interfaces()
	if err != nil {
		return "localhost"
	}

	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			var ip net.IP

			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}

			ipv4 := ip.To4()
			if ipv4 == nil || ipv4.IsLoopback() {
				continue
			}

			return ipv4.String()
		}
	}

	return "localhost"
}

// startLANBroadcaster periodically sends a UDP multicast beacon so that
// `termchat discover` on the same LAN can find this host.
func startLANBroadcaster(room string, port int, hostNick string) {
	beacon := lanBeacon{
		Room: room,
		Port: port,
		Host: hostNick,
		IP:   GetLocalIP(),
	}

	payload, err := json.Marshal(beacon)
	if err != nil {
		return
	}

	msg := []byte(fmt.Sprintf("%s|%s", shared.DiscoveryMagic, payload))

	addr := &net.UDPAddr{
		IP:   net.ParseIP(shared.DiscoveryMulticast),
		Port: shared.DiscoveryPort,
	}

	go func() {
		for {
			conn, err := net.DialUDP("udp4", nil, addr)
			if err != nil {
				time.Sleep(2 * time.Second)
				continue
			}

			conn.Write(msg)
			conn.Close()

			time.Sleep(1 * time.Second)
		}
	}()
}
