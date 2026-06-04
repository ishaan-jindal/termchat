package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"termchat/shared"
)

type discoverOptions struct {
	Online bool
	Local  bool
	API    string
}

func runDiscover(opts discoverOptions) {
	showOnline := opts.Online || (!opts.Online && !opts.Local)
	showLocal := opts.Local || (!opts.Online && !opts.Local)

	if showOnline {
		discoverOnline(opts.API)
	}

	if showLocal {
		if showOnline {
			fmt.Println()
		}
		discoverLAN()
	}
}

// --- Online discovery ---

func discoverOnline(apiURL string) {
	fmt.Println("╔══════════════════════════════════════╗")
	fmt.Println("║         ☁  ONLINE ROOMS              ║")
	fmt.Println("╚══════════════════════════════════════╝")

	client := &http.Client{Timeout: 5 * time.Second}

	resp, err := client.Get(apiURL + "/discover")
	if err != nil {
		fmt.Println("  Could not reach server:", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK ||
		!strings.Contains(resp.Header.Get("Content-Type"), "application/json") {
		fmt.Println("  Server does not support discovery yet.")
		fmt.Println("  Deploy the latest server to enable online room discovery.")
		return
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Println("  Error reading response:", err)
		return
	}

	var rooms []shared.RoomInfo

	err = json.Unmarshal(body, &rooms)
	if err != nil {
		fmt.Println("  Error parsing response:", err)
		return
	}

	if len(rooms) == 0 {
		fmt.Println("  No rooms found.")
		return
	}

	fmt.Println()
	fmt.Printf("  %-8s %-10s %-8s %s\n", "ROOM", "HOST", "USERS", "STATUS")
	fmt.Printf("  %-8s %-10s %-8s %s\n",
		strings.Repeat("─", 6),
		strings.Repeat("─", 8),
		strings.Repeat("─", 5),
		strings.Repeat("─", 10))

	for _, room := range rooms {
		status := "🔓 open"
		if room.HasPassword {
			status = "🔒 locked"
		}

		host := room.HostNick
		if host == "" {
			host = "—"
		}

		if len(host) > 8 {
			host = host[:8]
		}

		fmt.Printf("  %-8s %-10s %-8d %s\n", room.ID, host, room.UserCount, status)
	}

	fmt.Println()
	fmt.Println("  Join with: termchat <ROOM>")
}

// --- LAN discovery ---

type lanBeacon struct {
	Room string `json:"room"`
	Port int    `json:"port"`
	Host string `json:"host"`
	IP   string `json:"ip"`
}

func discoverLAN() {
	fmt.Println("╔══════════════════════════════════════╗")
	fmt.Println("║         📡  LAN ROOMS                ║")
	fmt.Println("╚══════════════════════════════════════╝")
	fmt.Println("  Scanning local network...")

	beacons := listenForBeacons(3 * time.Second)

	if len(beacons) == 0 {
		fmt.Println("  No LAN rooms found.")
		return
	}

	fmt.Println()
	fmt.Printf("  %-8s %-10s %-18s %s\n", "ROOM", "HOST", "ADDRESS", "PORT")
	fmt.Printf("  %-8s %-10s %-18s %s\n",
		strings.Repeat("─", 6),
		strings.Repeat("─", 8),
		strings.Repeat("─", 16),
		strings.Repeat("─", 5))

	for _, b := range beacons {
		host := b.Host
		if host == "" {
			host = "—"
		}
		if len(host) > 8 {
			host = host[:8]
		}

		fmt.Printf("  %-8s %-10s %-18s %d\n", b.Room, host, b.IP, b.Port)
	}

	fmt.Println()
	fmt.Println("  Join with: termchat <ROOM> --host <ADDRESS>")
}

func listenForBeacons(timeout time.Duration) []lanBeacon {
	addr := &net.UDPAddr{
		IP:   net.ParseIP(shared.DiscoveryMulticast),
		Port: shared.DiscoveryPort,
	}

	conn, err := net.ListenMulticastUDP("udp4", nil, addr)
	if err != nil {
		fmt.Println("  Error listening for LAN beacons:", err)
		return nil
	}
	defer conn.Close()

	conn.SetReadDeadline(time.Now().Add(timeout))

	seen := map[string]bool{}
	var results []lanBeacon
	buf := make([]byte, 1024)

	for {
		n, src, err := conn.ReadFromUDP(buf)
		if err != nil {
			break
		}

		data := string(buf[:n])

		// Expect: TERMCHAT_DISCOVER|<json>
		if !strings.HasPrefix(data, shared.DiscoveryMagic+"|") {
			continue
		}

		jsonData := data[len(shared.DiscoveryMagic)+1:]

		var beacon lanBeacon
		if err := json.Unmarshal([]byte(jsonData), &beacon); err != nil {
			continue
		}

		// Use the source IP if beacon doesn't have one
		if beacon.IP == "" {
			beacon.IP = src.IP.String()
		}

		key := fmt.Sprintf("%s:%d", beacon.IP, beacon.Port)
		if seen[key] {
			continue
		}
		seen[key] = true
		results = append(results, beacon)
	}

	return results
}
