package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"termchat/shared"

	"golang.org/x/net/ipv4"
)

type discoverOptions struct {
	Online bool
	Local  bool
	Base   string
}

func runDiscover(opts discoverOptions) {
	showOnline := opts.Online || (!opts.Online && !opts.Local)
	showLocal := opts.Local || (!opts.Online && !opts.Local)

	if showOnline {
		discoverOnline(opts.Base)
	}

	if showLocal {
		if showOnline {
			fmt.Println()
		}
		discoverLAN()
	}
}

// discoverBaseURL derives the HTTP base URL used for online discovery from
// the effective WebSocket server configuration.
func discoverBaseURL(opts cliOptions) string {
	if opts.Host != "" {
		return fmt.Sprintf("http://%s:%d", opts.Host, opts.Port)
	}

	if opts.ServerSet {
		u, err := url.Parse(opts.Server)
		if err != nil || u.Host == "" {
			return DefaultBase
		}

		scheme := "https"
		if u.Scheme == "ws" {
			scheme = "http"
		}

		return scheme + "://" + u.Host
	}

	return DefaultBase
}

// --- Online discovery ---

func discoverOnline(apiURL string) {
	fmt.Println("========================================")
	fmt.Println("          ONLINE ROOMS")
	fmt.Println("========================================")

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
		strings.Repeat("-", 6),
		strings.Repeat("-", 8),
		strings.Repeat("-", 5),
		strings.Repeat("-", 10))

	for _, room := range rooms {
		status := "[open]"
		if room.HasPassword {
			status = "[locked]"
		}

		host := room.HostNick
		if host == "" {
			host = "-"
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
	fmt.Println("========================================")
	fmt.Println("           LAN ROOMS")
	fmt.Println("========================================")
	fmt.Println("  Scanning local network...")

	beacons := listenForBeacons(3 * time.Second)

	if len(beacons) == 0 {
		fmt.Println("  No LAN rooms found.")
		fmt.Println()
		fmt.Println("  LAN discovery only sees hosts on the same network segment;")
		fmt.Println("  routers, hotspots, and AP isolation block it.")
		fmt.Println("  Join directly instead: termchat <ROOM> --host <ADDRESS> --port <PORT>")
		return
	}

	fmt.Println()
	fmt.Printf("  %-8s %-10s %-18s %s\n", "ROOM", "HOST", "ADDRESS", "PORT")
	fmt.Printf("  %-8s %-10s %-18s %s\n",
		strings.Repeat("-", 6),
		strings.Repeat("-", 8),
		strings.Repeat("-", 16),
		strings.Repeat("-", 5))

	for _, b := range beacons {
		host := b.Host
		if host == "" {
			host = "-"
		}
		if len(host) > 8 {
			host = host[:8]
		}

		fmt.Printf("  %-8s %-10s %-18s %d\n", b.Room, host, b.IP, b.Port)
	}

	fmt.Println()
	fmt.Println("  Join with: termchat <ROOM> --host <ADDRESS> --port <PORT>")
}

func listenForBeacons(timeout time.Duration) []lanBeacon {
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{Port: shared.DiscoveryPort})
	if err != nil {
		fmt.Println("  Error listening for LAN beacons:", err)
		return nil
	}
	defer conn.Close()

	pc := ipv4.NewPacketConn(conn)

	group := &net.UDPAddr{IP: net.ParseIP(shared.DiscoveryMulticast)}

	for _, li := range lanInterfaces() {
		pc.JoinGroup(&li.iface, group)
	}

	conn.SetReadDeadline(time.Now().Add(timeout))

	seen := map[string]bool{}
	var results []lanBeacon
	buf := make([]byte, 1024)

	for {
		n, src, err := conn.ReadFromUDP(buf)
		if err != nil {
			break
		}

		beacon, ok := parseBeacon(buf[:n], src)
		if !ok {
			continue
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

// parseBeacon decodes TERMCHAT_DISCOVER|<json> payloads, falling back to the
// packet source address when the beacon carries no IP.
func parseBeacon(data []byte, src *net.UDPAddr) (lanBeacon, bool) {
	prefix := shared.DiscoveryMagic + "|"
	if !strings.HasPrefix(string(data), prefix) {
		return lanBeacon{}, false
	}

	var beacon lanBeacon

	err := json.Unmarshal(data[len(prefix):], &beacon)
	if err != nil {
		return lanBeacon{}, false
	}

	if beacon.IP == "" && src != nil {
		beacon.IP = src.IP.String()
	}

	if beacon.IP == "" || beacon.Port == 0 {
		return lanBeacon{}, false
	}

	return beacon, true
}
