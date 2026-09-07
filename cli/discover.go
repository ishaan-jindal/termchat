package main

import (
	"encoding/json"
	"errors"
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

// fetchOnlineRooms queries the server's /discover endpoint. The hub screen
// wraps it in a tea.Cmd so the request never blocks the TUI.
func fetchOnlineRooms(apiURL string) ([]shared.RoomInfo, error) {
	client := &http.Client{Timeout: 5 * time.Second}

	resp, err := client.Get(apiURL + "/discover")
	if err != nil {
		return nil, fmt.Errorf("Could not reach server: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK ||
		!strings.Contains(resp.Header.Get("Content-Type"), "application/json") {
		return nil, errors.New("Server does not support discovery yet.\n" +
			"Deploy the latest server to enable online room discovery.")
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("Error reading response: %w", err)
	}

	var rooms []shared.RoomInfo

	err = json.Unmarshal(body, &rooms)
	if err != nil {
		return nil, fmt.Errorf("Error parsing response: %w", err)
	}

	return rooms, nil
}

// --- LAN discovery ---

type lanBeacon struct {
	Room   string `json:"room"`
	Port   int    `json:"port"`
	Host   string `json:"host"`
	IP     string `json:"ip"`
	Locked bool   `json:"locked"`
}

func listenForBeacons(timeout time.Duration) []lanBeacon {
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{Port: shared.DiscoveryPort})
	if err != nil {
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
