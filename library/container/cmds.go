package container

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

const (
	startingInNamespace string = "WORKERNATOR_NAMESPACED"
	setupNetNS          string = "WORKERNATOR_SETUP_NET_NS"
	setupVeth           string = "WORKERNATOR_SETUP_VETH"
)

// SetRootCommandName ...
func (wr *Wrangler) SetRootCommandName(cmdName string) {
	wr.commandRoot = cmdName
}

// RunContainer ...
func RunContainer() *cobra.Command {
	var pidLimit int
	var memLimit int
	var cpuWeight int
	var cpuMax int
	var cpuPeriod int
	var ioMbps int
	var ioIops int

	cmd := &cobra.Command{
		Use:          "run [librarypath] [runpath] [image] [command] [args...]",
		Short:        "for testing running containers",
		SilenceUsage: true,
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
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

			if ioMbps > 0 {
				args = append(args, "--ioMbps", strconv.Itoa(ioMbps))
			}

			if ioIops > 0 {
				args = append(args, "--ioIops", strconv.Itoa(ioIops))
			}

			cont.SetArgs(args)
			cont.SetStdErr(cmd.ErrOrStderr())
			cont.SetStdOut(cmd.OutOrStdout())

			if err := cont.Run(cmd.Context()); err != nil {
				_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "container failed to run: %v\n", err)
			}

			exitCode, err := cont.Wait(cmd.Context())
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "container finished running, exit: %v %v\n", exitCode, err)

			return nil
		},
	}

	cmd.Flags().IntVar(&pidLimit, "pidLimit", -1, "sets the max number of pids in the container")
	cmd.Flags().IntVar(&memLimit, "memLimit", -1, "sets the max memory in MB for the container")
	cmd.Flags().IntVar(&cpuWeight, "cpuWeight", -1, "sets the weight of the CPU for the container")
	cmd.Flags().IntVar(&cpuMax, "cpuMax", -1, "must be set with --cpuPeriod, sets the max for CPU bandwidth")
	cmd.Flags().IntVar(&cpuPeriod, "cpuPeriod", -1, "must be set with --cpuMax, sets the period for CPU bandwidth")
	cmd.Flags().IntVar(&ioMbps, "ioMbps", -1, "sets max mb per second for io")
	cmd.Flags().IntVar(&ioIops, "ioIops", -1, "sets max iops for io")

	return cmd
}

// RunInNamespaceCmd generates the cobra command the server uses when
// launching a job in a container. This should be called in init() in
// the main package, and the returned command should be added to the
// root command
func RunInNamespaceCmd() *cobra.Command {
	var pidLimit int
	var memLimit int
	var cpuWeight int
	var cpuMax int
	var cpuPeriod int
	var ioMbps int
	var ioIops int

	cmd := &cobra.Command{
		Use:          startingInNamespace,
		Short:        "special command, do not use",
		Hidden:       true,
		SilenceUsage: true,
		PreRunE: func(cmd *cobra.Command, args []string) error {
			if len(args) < 5 {
				return fmt.Errorf("missing args, expect at least: containerID libDir runDir imageSource command [args...]")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			_, _ = fmt.Fprintf(os.Stdout, "this is where the command gets all set up so it's ready to run\n")

			_, _ = fmt.Fprintf(os.Stdout, "running as user:\n\teuid: %v\n\tuid: %v\n", os.Geteuid(), os.Getuid())

			conf := Config{
				LibPath: args[1],
				RunPath: args[2],
				TmpPath: "/tmp",
			}

			wrangler, err := NewWrangler(conf)
			if err != nil {
				return fmt.Errorf("unable to create wrangler: %w", err)
			}
			containerID := args[0]

			if err := wrangler.setupContainerNetworking(containerID); err != nil {
				return err
			}

			if err := wrangler.setupContainerCgroups(containerID); err != nil {
				return err
			}

			if pidLimit > 0 {
				err := wrangler.setContainerPidLimit(containerID, pidLimit)
				if err != nil {
					return fmt.Errorf("unable to set pid limit: %w", err)
				}
			}

			if memLimit > 0 {
				err := wrangler.setupContainerMemoryLimits(containerID, memLimit)
				if err != nil {
					return fmt.Errorf("unable to set memory limit: %w", err)
				}
			}

			if cpuWeight > 0 {
				err := wrangler.setupContainerCPUWeight(containerID, cpuWeight)
				if err != nil {
					return fmt.Errorf("unable to set cpu weight: %w", err)
				}
			}

			if cpuMax > 0 && cpuPeriod > 0 {
				err := wrangler.setupContainerCPUBandwidth(containerID, cpuMax, cpuPeriod)
				if err != nil {
					return fmt.Errorf("unable to set cpu bandwidth: %w", err)
				}
			}

			if ioMbps > 0 && ioIops > 0 {
				err := wrangler.setupContainerIOLimits(containerID, ioMbps, ioIops)
				if err != nil {
					return fmt.Errorf("unable to set io limits: %w", err)
				}
			}

			// okay, non-cgroup stuff now

			if err := wrangler.copyNameserverConfig(containerID); err != nil {
				return err
			}

			if err := wrangler.mountProc(containerID); err != nil {
				return err
			}

			// if err := wrangler.chrootContainer(containerID); err != nil {
			// 	return err
			// }

			if err := wrangler.mountContainerDirectories(containerID); err != nil {
				if unmErr := wrangler.umountContainerDirectories(containerID); unmErr != nil {
					_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "unable to unmount container directories: %v\n", unmErr)
				}
				return err
			}

			if err := wrangler.pivotRoot(containerID); err != nil {
				return err
			}

			// if err := wrangler.setupLocalInterface(containerID); err != nil {
			// 	if unmErr := wrangler.umountContainerDirectories(containerID); unmErr != nil {
			// 		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "unable to unmount container directories: %v\n", unmErr)
			// 	}
			// 	return err
			// }

			var commandToRun string
			var commandArgs []string
			if len(args) == 5 {
				commandToRun = args[4]
			}
			if len(args) > 5 {
				commandToRun = args[4]
				commandArgs = args[5:]
			}

			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "container id: %v\n", containerID)
			for i, c := range commandArgs {
				if strings.Contains(c, "%%CONTAINERID%%") {
					commandArgs[i] = strings.Replace(c, "%%CONTAINERID%%", containerID, -1)
				}
			}

			runCmd := exec.Command(commandToRun, commandArgs...)
			runCmd.Stdout = cmd.OutOrStdout()
			runCmd.Stderr = cmd.ErrOrStderr()
			runCmd.Env = []string{
				"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
			}

			wrangler.debugLog("this is the command running now: \n\n--------------------------------------------------\n")
			runErr := runCmd.Run()
			wrangler.debugLog("\n--------------------------------------------------\n\ncontainer done running\n")

			if err := wrangler.umountContainerDirectories(containerID); err != nil {
				_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "unable to unmount container directories: %v\n", err)
			}

			return runErr
		},
	}

	cmd.Flags().IntVar(&pidLimit, "pidLimit", -1, "sets the max number of pids in the container")
	cmd.Flags().IntVar(&memLimit, "memLimit", -1, "sets the max memory in MB for the container")
	cmd.Flags().IntVar(&cpuWeight, "cpuWeight", -1, "sets the weight of the CPU for the container")
	cmd.Flags().IntVar(&cpuMax, "cpuMax", -1, "must be set with --cpuPeriod, sets the max for CPU bandwidth")
	cmd.Flags().IntVar(&cpuPeriod, "cpuPeriod", -1, "must be set with --cpuMax, sets the period for CPU bandwidth")
	cmd.Flags().IntVar(&ioMbps, "ioMbps", -1, "sets max mb per second for io")
	cmd.Flags().IntVar(&ioIops, "ioIops", -1, "sets max iops for io")
	return cmd
}

// // LaunchJobCmd handles the the last bits of setup before running the actual
// // 'job' command the user has requested.
// func LaunchJobCmd() *cobra.Command {
// 	return &cobra.Command{
// 		Use:    finalRun,
// 		Short:  "special command, do not use",
// 		Hidden: true,
// 		PreRunE: func(cmd *cobra.Command, args []string) error {
// 			return nil
// 		},
// 		RunE: func(cmd *cobra.Command, args []string) error {
// 			spew.Dump(args)
// 			fmt.Fprintf(os.Stdout, "this is where some final setup happens, and then the /actual/ job command gets run\n")
// 			return nil
// 		},
// 	}
// }

// SetupNetNS ...
func SetupNetNS() *cobra.Command {
	cmd := &cobra.Command{
		Use:          setupNetNS,
		SilenceUsage: true,
		Short:        "special command to setup network namespace",
		Hidden:       true,
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			wr, err := getWrangler(cmd, args)
			if err != nil {
				return err
			}

			fmt.Fprintf(cmd.OutOrStdout(), "handling network namespace setup: %v\n", args)

			return wr.setupNetworkNamespace()
		},
	}

	return cmd
}

// SetupVeth ...
func SetupVeth() *cobra.Command {
	cmd := &cobra.Command{
		Use:          setupVeth,
		SilenceUsage: true,
		Short:        "special command to setup veth devices",
		Hidden:       true,
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			wr, err := getWrangler(cmd, args)
			if err != nil {
				return err
			}

			fmt.Fprintf(cmd.OutOrStdout(), "handling veth setup: %v\n", args)
			return wr.setupNetworkVeth()
		},
	}

	return cmd
}

func getWrangler(cmd *cobra.Command, args []string) (*Wrangler, error) {
	if len(args) < 5 {
		return nil, fmt.Errorf("subcommand expects at least 5 arguments, got: %v (%v)", len(args), args)
	}

	conf := Config{
		LibPath: args[1],
		RunPath: args[2],
		TmpPath: args[3],
	}

	wr, err := NewWrangler(conf)

	wr.processingConntainer = true
	wr.containerID = args[4]

	return wr, err
}
