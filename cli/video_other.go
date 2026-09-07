//go:build !linux

package main

func platformCamInputs(_ string) [][]string {
	return nil
}
