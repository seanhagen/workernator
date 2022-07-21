package container

import (
	cryptoRand "crypto/rand"
	"fmt"
	mathRand "math/rand"
	"net"
	"os"
	"syscall"

	"github.com/davecgh/go-spew/spew"
	"github.com/vishvananda/netlink"
	"golang.org/x/sys/unix"
)

const bridgeName string = "workernator0"
const bridgeAddr = "172.16.0.1"

func (wr *Wrangler) isBridgeUp(links []netlink.Link) bool {
	for _, link := range links {
		if link.Type() == "bridge" && link.Attrs().Name == bridgeName {
			return true
		}
	}

	return false
}

/*
	This function sets up the "workernator00" bridge, which is our main bridge
	interface. To keep things simple, we assign the hopefully unassigned
	and obscure private IP 172.16.0.1 to it, which is from the range of
	IPs which we will also use for our containers.
*/
func (wr *Wrangler) setupBridge() error {
	wr.debugLog("setting up bridge\n")
	linkAttrs := netlink.NewLinkAttrs()
	linkAttrs.Name = bridgeName
	bridge := &netlink.Bridge{LinkAttrs: linkAttrs}
	if err := netlink.LinkAdd(bridge); err != nil {
		return fmt.Errorf("unable to add bridge '%v': %w", bridgeName, err)
	}

	addr, err := netlink.ParseAddr(bridgeAddr + "/16")
	if err != nil {
		return fmt.Errorf("unable to parse network address '%v/16': %w", bridgeAddr, err)
	}

	if err := netlink.AddrAdd(bridge, addr); err != nil {
		return fmt.Errorf("unable to add address: %w", err)
	}

	if err := netlink.LinkSetUp(bridge); err != nil {
		return fmt.Errorf("unable to set link status to 'up': %w", err)
	}

	return nil
}

// setupVirtualEthOnHost ...
func (wr *Wrangler) setupVirtualEthOnHost(ct *Container) error {
	veth0 := "veth0_" + ct.id.String()[:6]
	veth1 := "veth1_" + ct.id.String()[:6]

	linkAttrs := netlink.NewLinkAttrs()
	linkAttrs.Name = veth0

	veth0Struct := &netlink.Veth{
		LinkAttrs:        linkAttrs,
		PeerName:         veth1,
		PeerHardwareAddr: createMACAddress(),
	}
	if err := netlink.LinkAdd(veth0Struct); err != nil {
		spew.Dump(linkAttrs, veth0Struct)
		return fmt.Errorf("unable to add link: %w", err)
	}

	if err := netlink.LinkSetUp(veth0Struct); err != nil {
		return fmt.Errorf("unable to setup link: %w", err)
	}
	linkBridge, _ := netlink.LinkByName("workernator0")
	if err := netlink.LinkSetMaster(veth0Struct, linkBridge); err != nil {
		return fmt.Errorf("unable to setup link master: %w", err)
	}

	return nil
}

// setupNetworkNamespace ...
func (wr *Wrangler) setupNetworkNamespace() error {
	if err := mkdirIfNotExist(wr.pathToNetNs()); err != nil {
		return err
	}

	nsMount := wr.pathToNSMount()
	if _, err := unix.Open(nsMount, unix.O_RDONLY|unix.O_CREAT|unix.O_EXCL, 0644); err != nil {
		return fmt.Errorf("unable to open bind mount: %w", err)
	}

	fd, err := unix.Open("/proc/self/ns/net", unix.O_RDONLY, 0)
	defer unix.Close(fd)
	if err != nil {
		return fmt.Errorf("unable to open /proc/self/ns/net: %w", err)
	}

	if err := unix.Unshare(unix.CLONE_NEWNET); err != nil {
		return fmt.Errorf("unshare system call failed: %w", err)
	}
	if err := unix.Mount("/proc/self/ns/net", nsMount, "bind", unix.MS_BIND, ""); err != nil {
		return fmt.Errorf("mount system call failed: %w", err)
	}
	if err := unix.Setns(fd, unix.CLONE_NEWNET); err != nil {
		return fmt.Errorf("setns system call failed: %w", err)
	}

	return nil
}

// setupNetworkVeth ...
func (wr *Wrangler) setupNetworkVeth() error {
	if err := mkdirIfNotExist(wr.pathToNetNs()); err != nil {
		return err
	}

	nsMount := wr.pathToNSMount()
	fd, err := unix.Open(nsMount, unix.O_RDONLY, 0)
	defer unix.Close(fd)
	if err != nil {
		return fmt.Errorf("unable to open ns mount '%v': %w", nsMount, err)
	}

	// Set veth1 of the new container to the new network namespace
	veth1 := wr.veth1Name()
	// fmt.Printf("setting veth1 to '%v'", veth1)

	// fmt.Printf("getting link by name\n")
	veth1Link, err := netlink.LinkByName(veth1)
	if err != nil {
		return fmt.Errorf("unable to fetch veth1 (%v): %w", veth1, err)
	}
	// fmt.Printf("done, now linking to namespace\n")
	if err := netlink.LinkSetNsFd(veth1Link, fd); err != nil {
		return fmt.Errorf("unable to set network namespace for veth1: %w", err)
	}

	// fmt.Printf("setting namespace to net ns...")
	if err := unix.Setns(fd, unix.CLONE_NEWNET); err != nil {
		return fmt.Errorf("setns system call failed: %w", err)
	}

	// fmt.Printf("setting ip address for link...")
	addr, _ := netlink.ParseAddr(createIPAddress() + "/16")
	if err := netlink.AddrAdd(veth1Link, addr); err != nil {
		return fmt.Errorf("unable to assign IP '%v' to veth1: %w", addr.String(), err)
	}
	// fmt.Printf("done!\n")

	// fmt.Printf("bringing up the interface...")
	if err := netlink.LinkSetUp(veth1Link); err != nil {
		return fmt.Errorf("unable to bring up veth1: %w", err)
	}

	// fmt.Printf("adding default route...")
	route := netlink.Route{
		Scope:     netlink.SCOPE_UNIVERSE,
		LinkIndex: veth1Link.Attrs().Index,
		Gw:        net.ParseIP(bridgeAddr),
		Dst:       nil,
	}

	if err := netlink.RouteAdd(&route); err != nil {
		wr.debugLog("uanble to add default route '%v': %v", bridgeAddr, err)
		spew.Dump(route)
		return fmt.Errorf("unable to add default route: %w", err)
	}

	// fmt.Printf(" done!\n")
	return nil
}

// setupContainerNetworking ...
func (wr *Wrangler) setupContainerNetworking(containerID string) error {
	err := syscall.Sethostname([]byte("workernator-" + containerID))
	if err != nil {
		return fmt.Errorf("unable to set hostname: %w", err)
	}

	// // don't need this because the CLONE_NEWNET flag is set when the container runs
	// nsMount := wr.pathToNSMount() + "/" + containerID
	// fd, err := unix.Open(nsMount, unix.O_RDONLY, 0)
	// if err != nil {
	// 	return fmt.Errorf("unable to open network ns mount: %w", err)
	// }

	// err = unix.Setns(fd, unix.CLONE_NEWNET)
	// if err != nil {
	// 	return fmt.Errorf("setns system call failed: %w", err)
	// }

	return nil
}

// copyNameserverConfig  ...
func (wr *Wrangler) copyNameserverConfig(containerID string) error {
	resolvFilePaths := []string{
		"/var/run/systemd/resolve/resolv.conf",
		"/etc/gockerresolv.conf",
		"/etc/resolv.conf",
	}

	for _, resolvFilePath := range resolvFilePaths {
		if _, err := os.Stat(resolvFilePath); os.IsNotExist(err) {
			continue
		} else {
			return wr.copyFile(resolvFilePath,
				wr.getContainerFSHome(containerID)+"/mnt/etc/resolv.conf")
		}
	}
	return nil
}

// setupLocalInterface  ...
func (wr *Wrangler) setupLocalInterface(containerID string) error {
	links, _ := netlink.LinkList()
	for _, link := range links {
		if link.Attrs().Name == "lo" {
			loAddr, _ := netlink.ParseAddr("127.0.0.1/32")
			if err := netlink.AddrAdd(link, loAddr); err != nil {
				wr.debugLog("unable to configure local interface: %v\n", err)
			}
			if err := netlink.LinkSetUp(link); err != nil {
				wr.debugLog("unable to set link '%v' status to up: %v\n", link.Attrs().Name, err)
			}
		}
	}

	return nil
}

func createMACAddress() net.HardwareAddr {
	hw := make(net.HardwareAddr, 6)
	hw[0] = 0x02
	hw[1] = 0x42
	_, _ = cryptoRand.Read(hw[2:])
	return hw
}

func createIPAddress() string {
	byte1 := mathRand.Intn(254)
	byte2 := mathRand.Intn(254)
	return fmt.Sprintf("172.16.%d.%d", byte1, byte2)
}

// veth1Name ...
func (wr *Wrangler) veth1Name() string {
	return "veth1_" + wr.containerID[:6]
}
