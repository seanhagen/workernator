package container

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"syscall"

	"github.com/rs/xid"
	"golang.org/x/sys/unix"
)

// const (
// 	envJobID = "ENV_WORKERNATOR_CONTAINER_ID"
// )

// Container ...
type Container struct {
	id  xid.ID
	img *Image

	cmd    *exec.Cmd
	done   context.Context
	cancel context.CancelFunc

	lock      sync.Mutex
	exitCode  int
	exitError error

	baseCommand       string
	commandToRun      string
	commandArgs       []string
	stdout            io.Writer
	stderr            io.Writer
	pathToContainerFs string
	pathToRunDir      string
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
func (c *Container) Run(ctx context.Context) error {
	c.done, c.cancel = context.WithCancel(ctx)

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
			[]string{"/proc/self/exe", startingInNamespace, c.id.String()}, // c.baseCommand,startingInNamespace},
			c.commandArgs...,
		),
		Stdout: stdout,
		Stderr: stderr,

		SysProcAttr: &syscall.SysProcAttr{
			Pdeathsig: unix.SIGTERM,
			Cloneflags: syscall.CLONE_NEWUSER |
				syscall.CLONE_NEWUTS |
				syscall.CLONE_NEWNS |
				syscall.CLONE_NEWPID |
				unix.CLONE_NEWNET,

			// Unshareflags: syscall.CLONE_NEWNS,
			Unshareflags: syscall.CLONE_NEWNS |
				unix.CLONE_NEWCGROUP |
				unix.CLONE_NEWUTS |
				unix.CLONE_NEWNET,

			// UidMappings: []syscall.SysProcIDMap{
			// 	{ContainerID: 0, HostID: 1000, Size: 1},
			// },
			UidMappings: []syscall.SysProcIDMap{
				{ContainerID: 0, HostID: os.Getuid(), Size: 1},
			},

			GidMappings: []syscall.SysProcIDMap{
				{ContainerID: 0, HostID: os.Getgid(), Size: 1},
			},
		},
	}
	c.cmd = cmd

	if err := cmd.Start(); err != nil {
		return err
	}

	// waits for the command to finish, then umounts the fs & net ns,
	// then removes container from cgroups, then removes the container directory
	go c.cleanupWhenDone()

	return nil
}

// cleanupWhenDone ...
func (c *Container) cleanupWhenDone() {
	c.exitError = c.cmd.Wait()
	c.exitCode = c.cmd.ProcessState.ExitCode()
	c.cancel()

	_, _ = fmt.Fprintf(os.Stdout, "umounting network namespace\n")
	if err := unmountNetworkNamespace(c.id.String(), c.pathToRunDir); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "unable to unmount network namespace: %v\n", err)
	}

	_, _ = fmt.Fprintf(os.Stdout, "umounting container fs\n")
	if err := unmountContainerFS(c.id.String(), c.pathToRunDir); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "unable to umount container filesystem: %v\n", err)
	}

	_, _ = fmt.Fprintf(os.Stdout, "removing container from cgroups\n")
	if err := removeContainerCGroups(c.id.String()); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "unable to remove container cgroups: %v\n", err)
	}

	_, _ = fmt.Fprintf(os.Stdout, "removing container fs directory\n")
	if err := os.RemoveAll(c.pathToContainerFs); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "unable to remove container dir '%v': %v", c.pathToContainerFs, err)
	}
}

// Wait  ...
func (c *Container) Wait(ctx context.Context) (int, error) {
	if c.cmd == nil {
		return 0, fmt.Errorf("container hasn't started yet")
	}

	select {
	case <-c.done.Done():
		// job completed, container signaled itself it's done
	case <-ctx.Done():
		// context cancled, don't wait any more
		return -1, ctx.Err()
	}

	<-c.done.Done()
	c.lock.Lock()
	err, exit := c.exitError, c.exitCode
	c.lock.Unlock()

	return exit, err
}

// Kill  ...
func (c *Container) Kill() error {
	return c.cmd.Process.Signal(unix.SIGTERM)
}
