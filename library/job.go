package workernator

import (
	"fmt"
	"io"
	"os/exec"
	"syscall"

	"github.com/rs/xid"
	"golang.org/x/sys/unix"
)

const defaultRootFSPath = "/tmp/workernator/jobfs"

const (
	runInNamespace = "00000-run-in-ns-00000"
	envJobID       = "ENV_WORKERNATOR_JOB_ID"
	envRootFSPath  = "ENV_WORKERNATOR_JOB_ROOTFS"
)

const (
	wrkHomePath       = "/var/lib/workernator"
	wrkTempPath       = wrkHomePath + "/tmp"
	wrkImgsPath       = wrkHomePath + "/images"
	wkrContainersPath = "/var/run/workernator/containers"
	wrkNetNsPath      = "/var/run/workernator/net-ns"
)

// Job ...
type Job struct {
	id  xid.ID
	cmd *exec.Cmd
}

// NewJob ...
func NewJob(input io.Reader, output, errors io.Writer, toRun string, args []string, rootFSPath string) (*Job, error) {
	//   when calling 'NewJob':
	//     - create container id
	job := Job{id: xid.New()}

	//     - create required container directories
	//     - mount overlay file system
	//     - setup virtual eth on host

	if rootFSPath == "default" {
		rootFSPath = defaultRootFSPath
	}

	if err := errorIfRootFSNotFound(rootFSPath); err != nil {
		return nil, err
	}

	job.cmd = &exec.Cmd{
		Path: "/proc/self/exe",
		Args: append([]string{runInNamespace, toRun}, args...),
		Env: []string{
			"PATH=/bin",
			fmt.Sprintf("%v=%v", envJobID, job.id.String()),
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

			// Unshareflags: syscall.CLONE_NEWNS |
			// 	unix.CLONE_NEWCGROUP |
			// 	unix.CLONE_NEWUTS,

			// UidMappings: []syscall.SysProcIDMap{
			// 	{ContainerID: 0, HostID: os.Getuid(), Size: 1},
			// },

			// GidMappings: []syscall.SysProcIDMap{
			// 	{ContainerID: 0, HostID: os.Getgid(), Size: 1},
			// },
		},
	}

	return &job, nil
}
