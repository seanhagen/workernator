package container

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/davecgh/go-spew/spew"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

const (
	startingInNamespace string = "__WORKERNATOR__NAMESPACED__"
	finalRun            string = "__WORKERNATOR__FINAL__"
	setupNetNS          string = "__SETUP_NET_NS__"
	setupVeth           string = "__SETUP_VETH__"
)

// SetRootCommandName ...
func (wr *Wrangler) SetRootCommandName(cmdName string) {
	wr.commandRoot = cmdName
}

// RunContainer ...
func RunContainer() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run [librarypath] [runpath] [image] [command] [args...]",
		Short: "for testing running containers",
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},

		RunE: func(cmd *cobra.Command, args []string) error {
			spew.Dump(args)

			conf := Config{
				LibPath: args[0],
				RunPath: args[1],
				TmpPath: "/tmp",
			}

			_, _ = fmt.Fprint(cmd.OutOrStdout(), "getting container wrangler\n")
			wrangler, err := NewWrangler(conf)
			if err != nil {
				return fmt.Errorf("couldn't create wrangler: %w", err)
			}

			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "getting image '%v'\n", args[2])
			img, err := wrangler.GetImage(cmd.Context(), args[2])
			if err != nil {
				return err
			}

			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "prepparing image for launch\n")
			cont, err := wrangler.PrepImageForLaunch(img)
			if err != nil {
				return fmt.Errorf("couldn't prepare container from image: %w", err)
			}

			_, _ = fmt.Fprintf(
				cmd.OutOrStdout(),
				"container ready, about to run '%v %v' in the container!\n",
				args[3], strings.Join(args[4:], " "))
			cont.SetCommand(startingInNamespace)
			cont.SetArgs(args[3:])
			cont.SetStdErr(cmd.ErrOrStderr())
			cont.SetStdOut(cmd.OutOrStdout())

			if err := cont.Run(); err != nil {
				return fmt.Errorf("container failed to run: %w", err)
			}

			return nil
		},
	}
	return cmd
}

// RunInNamespaceCmd generates the cobra command the server uses when
// launching a job in a container. This should be called in init() in
// the main package, and the returned command should be added to the
// root command
func RunInNamespaceCmd() *cobra.Command {
	var flagTest string

	cmd := &cobra.Command{
		Use:    startingInNamespace,
		Short:  "special command, do not use",
		Hidden: true,
		PreRunE: func(cmd *cobra.Command, args []string) error {
			setupLogger(cmd.OutOrStderr())

			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			spew.Dump(args, flagTest, os.Environ())
			fmt.Fprintf(os.Stdout, "this is where the command gets all set up so it's ready to run\n")
			return nil
		},
	}

	cmd.Flags().StringVarP(&flagTest, "flag", "f", "", "just testing")

	return cmd
}

// LaunchJobCmd handles the the last bits of setup before running the actual
// 'job' command the user has requested.
func LaunchJobCmd() *cobra.Command {
	return &cobra.Command{
		Use:    finalRun,
		Short:  "special command, do not use",
		Hidden: true,
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			setupLogger(cmd.OutOrStderr())

			spew.Dump(args)
			fmt.Fprintf(os.Stdout, "this is where some final setup happens, and then the /actual/ job command gets run\n")
			return nil
		},
	}
}

// SetupNetNS ...
func SetupNetNS() *cobra.Command {
	cmd := &cobra.Command{
		Use:    setupNetNS,
		Short:  "special command to setup network namespace",
		Hidden: true,
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			setupLogger(cmd.OutOrStderr())
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

// SetupVeth ...
func SetupVeth() *cobra.Command {
	cmd := &cobra.Command{
		Use:    setupVeth,
		Short:  "special command to setup veth devices",
		Hidden: true,
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			setupLogger(cmd.OutOrStderr())
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

func setupLogger(output io.Writer) {
	encConf := zapcore.EncoderConfig{
		MessageKey:     "msg",
		LevelKey:       "level",
		TimeKey:        "time",
		NameKey:        "logger",
		CallerKey:      "file",
		StacktraceKey:  "stacktrace",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.LowercaseLevelEncoder,
		EncodeTime:     zapcore.RFC3339TimeEncoder,
		EncodeDuration: zapcore.SecondsDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
		EncodeName:     zapcore.FullNameEncoder,
	}

	lvl := zap.NewAtomicLevel()
	lvl.SetLevel(zap.WarnLevel)

	isDev := strings.TrimSpace(os.Getenv("DEV_MODE"))
	if isDev != "" {
		lvl.SetLevel(zap.DebugLevel)
	}

	zc := zap.Config{
		Level:             lvl,
		DisableCaller:     false,
		DisableStacktrace: false,
		Encoding:          "console",
		EncoderConfig:     encConf,
		OutputPaths:       []string{"stdout"},
		ErrorOutputPaths:  []string{"stderr"},
	}

	opts := []zap.Option{
		//zap.AddCallerSkip(1),
	}

	log, err := zc.Build(opts...)
	if err != nil {
		_, _ = fmt.Fprintf(output, "unable to set up logging: %v", err)
	}
	zap.ReplaceGlobals(log)
}
