//go:build mage
// +build mage

package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/davecgh/go-spew/spew"
	"github.com/magefile/mage/sh"
)

const envPhysDevice string = "PHYS_DEVICE"

// EnableNetworking ...
func EnableNetworking() error {
	if os.Geteuid() != 0 {
		return fmt.Errorf("you must be root to run this command")
	}

	phys := strings.TrimSpace(os.Getenv(envPhysDevice))
	if phys == "" {
		return fmt.Errorf("you must set '%v' environment variable to be the physical device to connect", envPhysDevice)
	}

	if err := sh.Run("sysctl", "net.ipv4.conf.all.forwarding=1"); err != nil {
		return fmt.Errorf("unable to set forwarding: %w", err)
	}

	if err := sh.Run("iptables", "-P", "FORWARD", "ACCEPT"); err != nil {
		return fmt.Errorf("unable to set forwarding in iptables: %w", err)
	}

	err := sh.Run("iptables", "-t", "nat", "-A", "POSTROUTING", "-o", "workernator0", "-j", "MASQUERADE")
	if err != nil {
		return fmt.Errorf("unable to set postrouting nat: %w", err)
	}

	err = sh.Run("iptables", "-t", "nat", "-A", "POSTROUTING", "-o", phys, "-j", "MASQUERADE")
	if err != nil {
		return fmt.Errorf("unable to set routing to physical interface: %w", err)
	}
	return nil
}

// QuickTest ...
func QuickTest() error {
	spew.Dump(os.Args)
	return nil
}
