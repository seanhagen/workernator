package container

import "github.com/spf13/cobra"

const (
	setupNetNS string = "__SETUP_NET_NS__"
	setupVeth  string = "__SETUP_VETH__"
)

// AddRequiredSubcommands ...
func (wr *Wrangler) SetRootCommandName(cmdName string) {
	wr.commandRoot = cmdName
}

func SetupNetNS() *cobra.Command {
	cmd := &cobra.Command{
		Use:    setupNetNS,
		Short:  "special command to setup network namespace",
		Hidden: true,
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			// _ = createDirsIfDontExist([]string{getNetNsPath()})
			// nsMount := getNetNsPath() + "/" + containerID
			// if _, err := unix.Open(nsMount, unix.O_RDONLY|unix.O_CREAT|unix.O_EXCL, 0644); err != nil {
			// 	log.Fatalf("Unable to open bind mount file: :%v\n", err)
			// }

			// fd, err := unix.Open("/proc/self/ns/net", unix.O_RDONLY, 0)
			// defer unix.Close(fd)
			// if err != nil {
			// 	log.Fatalf("Unable to open: %v\n", err)
			// }

			// if err := unix.Unshare(unix.CLONE_NEWNET); err != nil {
			// 	log.Fatalf("Unshare system call failed: %v\n", err)
			// }
			// if err := unix.Mount("/proc/self/ns/net", nsMount, "bind", unix.MS_BIND, ""); err != nil {
			// 	log.Fatalf("Mount system call failed: %v\n", err)
			// }
			// if err := unix.Setns(fd, unix.CLONE_NEWNET); err != nil {
			// 	log.Fatalf("Setns system call failed: %v\n", err)
			// }

			return nil
		},
	}

	return cmd
}

func SetupVeth() *cobra.Command {
	cmd := &cobra.Command{
		Use:    setupVeth,
		Short:  "special command to setup veth devices",
		Hidden: true,
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			// nsMount := getNetNsPath() + "/" + containerID
			// fmt.Printf("opening net ns path: %v\n", nsMount)

			// fd, err := unix.Open(nsMount, unix.O_RDONLY, 0)
			// defer unix.Close(fd)
			// if err != nil {
			// 	log.Fatalf("Unable to open: %v\n", err)
			// }

			// /* Set veth1 of the new container to the new network namespace */
			// veth1 := "veth1_" + containerID[:6]
			// fmt.Printf("setting veth1 to '%v'", veth1)

			// fmt.Printf("getting link by name\n")
			// veth1Link, err := netlink.LinkByName(veth1)
			// if err != nil {
			// 	log.Fatalf("Unable to fetch veth1: %v\n", err)
			// }
			// fmt.Printf("done, now linking to namespace\n")
			// if err := netlink.LinkSetNsFd(veth1Link, fd); err != nil {
			// 	log.Fatalf("Unable to set network namespace for veth1: %v\n", err)
			// }
			// 		fmt.Printf("done!\n")

			// 			fmt.Printf("getting  net ns path...")
			// nsMount := getNetNsPath() + "/" + containerID
			// fmt.Printf(" done: %v\n", nsMount)

			// fmt.Printf("opening %v...", nsMount)
			// fd, err := unix.Open(nsMount, unix.O_RDONLY, 0)
			// defer unix.Close(fd)
			// if err != nil {
			// 	log.Fatalf("Unable to open: %v\n", err)
			// }
			// fmt.Printf("done!\n")

			// fmt.Printf("setting namespace to net ns...")
			// if err := unix.Setns(fd, unix.CLONE_NEWNET); err != nil {
			// 	log.Fatalf("Setns system call failed: %v\n", err)
			// }
			// fmt.Printf("done!\n")

			// fmt.Printf("getting veth1 & link...")
			// veth1 := "veth1_" + containerID[:6]
			// veth1Link, err := netlink.LinkByName(veth1)
			// if err != nil {
			// 	log.Fatalf("Unable to fetch veth1: %v\n", err)
			// }
			// fmt.Printf("done!\n")

			// fmt.Printf("setting ip address for link...")
			// addr, _ := netlink.ParseAddr(createIPAddress() + "/16")
			// if err := netlink.AddrAdd(veth1Link, addr); err != nil {
			// 	log.Fatalf("Error assigning IP to veth1: %v\n", err)
			// }
			// fmt.Printf("done!\n")

			// fmt.Printf("bringing up the interface...")
			// /* Bring up the interface */
			// doOrDieWithMsg(netlink.LinkSetUp(veth1Link), "Unable to bring up veth1")
			// fmt.Printf("done!\n")

			// fmt.Printf("adding default route...")
			// /* Add a default route */
			// route := netlink.Route{
			// 	Scope:     netlink.SCOPE_UNIVERSE,
			// 	LinkIndex: veth1Link.Attrs().Index,
			// 	Gw:        net.ParseIP("172.29.0.1"),
			// 	Dst:       nil,
			// }
			// doOrDieWithMsg(netlink.RouteAdd(&route), "Unable to add default route")
			// fmt.Printf(" done!\n")

			return nil
		},
	}

	return cmd
}
