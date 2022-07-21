package container

import (
	"fmt"
	"io"
	"os/exec"
	"syscall"

	"github.com/rs/xid"
	"golang.org/x/sys/unix"
)

const (
	envJobID = "ENV_WORKERNATOR_CONTAINER_ID"
)

// Container ...
type Container struct {
	id  xid.ID
	img *Image

	cmd *exec.Cmd

	baseCommand  string
	commandToRun string
	commandArgs  []string
	stdout       io.Writer
	stderr       io.Writer
}

// ID ...
func (c *Container) ID() xid.ID {
	return c.id
}

// SetCommandToRun  ...
func (c *Container) SetCommand(cmd string) {
	c.commandToRun = cmd
}

// Command  ...
func (c *Container) Command() string {
	return c.commandToRun
}

// SetArgs  ...
func (c *Container) SetArgs(args []string) {
	c.commandArgs = args
}

// Args ...
func (c *Container) Args() []string {
	return c.commandArgs
}

// SetStdOut ...
func (c *Container) SetStdOut(out io.Writer) {
	c.stdout = out
}

// SetStdErr ...
func (c *Container) SetStdErr(err io.Writer) {
	c.stderr = err
}

// Run  ...
func (c *Container) Run() error {
	stdout := io.Discard
	stderr := io.Discard

	if c.stdout != nil {
		stdout = c.stdout
	}

	if c.stderr != nil {
		stderr = c.stderr
	}

	cmd := &exec.Cmd{
		Path: "/proc/self/exe",
		Args: append(
			// startingInNamespace is the argument we give so that when this binary starts up
			//    it knows that it's suppposed to be running the specific subcommand
			//    that handles the fork/clone
			[]string{"/proc/self/exe", startingInNamespace}, // c.baseCommand,startingInNamespace},
			c.commandArgs...,
		),
		Env: []string{
			fmt.Sprintf("%v=%v", envJobID, c.id.String()),
		},
		Stdout: stdout,
		Stderr: stderr,

		SysProcAttr: &syscall.SysProcAttr{
			Pdeathsig: unix.SIGTERM,
			Cloneflags: syscall.CLONE_NEWUSER |
				syscall.CLONE_NEWUTS |
				syscall.CLONE_NEWNS |
				syscall.CLONE_NEWPID,
			UidMappings: []syscall.SysProcIDMap{
				{
					ContainerID: 0,
					HostID:      1000,
					Size:        1,
				},
			},
		},
	}
	c.cmd = cmd

	return cmd.Run()
}

// Wait  ...
func (c *Container) Wait() (int, error) {
	if c.cmd == nil {
		return 0, fmt.Errorf("container hasn't started yet")
	}

	err := c.cmd.Wait()
	exitCode := c.cmd.ProcessState.ExitCode()

	return exitCode, err
}

// Kill  ...
func (c *Container) Kill() error {
	return c.cmd.Process.Signal(unix.SIGTERM)
}

// LaunchContainer ...
//func (wr *Wrangler) LaunchContainer(container *Container) error {
/*
			var opts []string
			if mem > 0 {
				opts = append(opts, "--mem="+strconv.Itoa(mem))
			}
			if swap >= 0 {
				opts = append(opts, "--swap="+strconv.Itoa(swap))
			}
			if pids > 0 {
				opts = append(opts, "--pids="+strconv.Itoa(pids))
			}
			if cpus > 0 {
				opts = append(opts, "--cpus="+strconv.Itoa(cpus))
			}
			opts = append(opts, "--img="+imageShaHex)
			args := append([]string{containerID}, cmdArgs...)
			args = append(opts, args...)
			args = append([]string{"child-mode"}, args...)
			cmd = exec.Command("/proc/self/exe", args...)
			cmd.Stdin = os.Stdin
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			cmd.SysProcAttr = &unix.SysProcAttr{
				Cloneflags: unix.CLONE_NEWPID |
	      unix.CLONE_NEWUSER |
					unix.CLONE_NEWNS |
					unix.CLONE_NEWUTS |
					unix.CLONE_NEWIPC,
			}
			fmt.Printf("launching command %v for really reals\n", args)
			doOrDie(cmd.Run())
*/

// unmount network namespace

// umount container fs

// remove cgroups

// remove container folder
//}
