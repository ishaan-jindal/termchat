//go:build !linux

package main

func defaultRouteIface() string {
	return ""
}
