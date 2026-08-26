//go:build linux

package main

import (
	"os"
	"strconv"
	"strings"
)

// defaultRouteIface returns the interface owning the IPv4 default route by
// reading /proc/net/route, or "" when none is present.
func defaultRouteIface() string {
	data, err := os.ReadFile("/proc/net/route")
	if err != nil {
		return ""
	}

	for _, line := range strings.Split(string(data), "\n")[1:] {
		fields := strings.Fields(line)

		if len(fields) < 4 {
			continue
		}

		if fields[1] != "00000000" {
			continue
		}

		flags, err := strconv.ParseUint(fields[3], 16, 32)
		if err != nil {
			continue
		}

		if flags&0x1 != 0 {
			return fields[0]
		}
	}

	return ""
}
