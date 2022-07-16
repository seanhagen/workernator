package workernator

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"syscall"

	"github.com/rs/xid"
	"golang.org/x/sys/unix"
)

const defaultRootFSPath = "/tmp/workernator/containerfs"

const (
	runInNamespace = "$$ns$$"
	envContainerID = "ENV_WORKERNATOR_JOB_ID"
	envRootFSPath  = "ENV_WORKERNATOR_JOB_ROOTFS"
)

// Container ...
type Container struct {
	id      xid.ID
	command string
	cmd     *exec.Cmd
}

// NewContainer ...
func NewContainer(input io.Reader, output, errors io.Writer, toRun string, args []string, rootFSPath string) (*Container, error) {
	id := xid.New()

	if rootFSPath == "default" {
		rootFSPath = defaultRootFSPath
	}

	if err := errorIfRootFSNotFound(rootFSPath); err != nil {
		return nil, err
	}

	cmd := &exec.Cmd{
		Path: "/proc/self/exe",
		Args: append([]string{runInNamespace, toRun}, args...),
		Env: []string{
			"PATH=/bin",
			fmt.Sprintf("%v=%v", envContainerID, id.String()),
			fmt.Sprintf("%v=%v", envRootFSPath, rootFSPath),
		},

		Stdin:  input,
		Stdout: output,
		Stderr: errors,

		SysProcAttr: &syscall.SysProcAttr{
			Pdeathsig: unix.SIGTERM,
			Cloneflags: syscall.CLONE_NEWUSER |
				syscall.CLONE_NEWNS |
				syscall.CLONE_NEWPID,

			Unshareflags: syscall.CLONE_NEWNS |
				unix.CLONE_NEWCGROUP |
				unix.CLONE_NEWUTS,

			UidMappings: []syscall.SysProcIDMap{
				{ContainerID: 0, HostID: os.Getuid(), Size: 1},
			},

			GidMappings: []syscall.SysProcIDMap{
				{ContainerID: 0, HostID: os.Getgid(), Size: 1},
			},
		},
	}

	return &Container{id: id, command: toRun, cmd: cmd}, nil
}

// Run ...
func (c *Container) Run(ctx context.Context) error {
	return c.cmd.Run()
}

// Run ...
func (c *Container) Start(ctx context.Context) error {
	return c.cmd.Start()
}

// Stop ...
func (c *Container) Stop(ctx context.Context) error {
	return c.cmd.Process.Kill()
}

// Wait ...
func (c *Container) Wait(ctx context.Context) error {
	return c.cmd.Wait()
}

func runningInContainer() error {
	id := os.Getenv(envContainerID)
	rfs := os.Getenv(envRootFSPath)

	syscall.Sethostname([]byte(id))
	cg(fmt.Sprintf("workernator-container-%v", id))
	mountProc(rfs)
	pivotRoot(rfs)

	if err := os.Chdir("/"); err != nil {
		return fmt.Errorf("unable to chdir to new root: %w", err)
	}

	cmd := exec.Command(os.Args[1], os.Args[2:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		fmt.Printf("unable to run command: %v\n", err)
		os.Exit(1)
	}

	return nil
}
