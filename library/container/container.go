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
			Cloneflags: syscall.CLONE_NEWNS | syscall.CLONE_NEWUTS |
				syscall.CLONE_NEWIPC | syscall.CLONE_NEWPID |
				syscall.CLONE_NEWNET | syscall.CLONE_NEWUSER,

			Unshareflags: syscall.CLONE_NEWNS,

			UidMappings: []syscall.SysProcIDMap{
				{ContainerID: 0, HostID: os.Getuid(), Size: 1},
			},

			GidMappings: []syscall.SysProcIDMap{
				{ContainerID: 0, HostID: os.Getgid(), Size: 1},
			},
			GidMappingsEnableSetgroups: true,

			// want CAP_MKNOD
			// include/uapi/linux/capability.h says it's 27?
			//AmbientCaps: []uintptr{27, 21, 18},
			AmbientCaps: []uintptr{8, 18, 21, 27},
		},
	}
	c.cmd = cmd

	_, _ = fmt.Fprintf(c.stdout, "container hasn't run yet\n")
	if err := cmd.Start(); err != nil {
		_, _ = fmt.Fprintf(c.stdout, "container encountered error trying to start: %v\n", err)
		c.cancel()
		return err
	}
	_, _ = fmt.Fprintf(c.stdout, "container running!\n")

	// waits for the command to finish, then umounts the fs & net ns,
	// then removes container from cgroups, then removes the container directory
	go c.cleanupWhenDone()

	return nil
}

// cleanupWhenDone ...
func (c *Container) cleanupWhenDone() {
	c.exitError = c.cmd.Wait()
	c.exitCode = c.cmd.ProcessState.ExitCode()

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
	c.cancel()
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
