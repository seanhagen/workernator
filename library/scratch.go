package workernator

import (
	"fmt"
	"io/fs"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"syscall"
)

const cgroupPathFmt = "/sys/fs/cgroup/%v/workernator/%v"
const nsBase = 1000000

var defaultCGroups = []string{"cpu", "memory", "pids"}

func createCGroups(containerID string) error {
	cgroups := generateCGroupList(containerID, defaultCGroups)

	if err := createDirsIfMissing(cgroups); err != nil {
		return fmt.Errorf("unable to create cgroup directories: %w", err)
	}

	for _, cdir := range cgroups {
		initializeCgroup(cdir)
	}

	return nil
}

func initializeCgroup(cgroupDir string) error {
	err := writeNumberToFile(cgroupDir+"/notify_on_release", 1, 0700)
	if err != nil {
		return fmt.Errorf("unable to write cgroup notification file for '%v': %w", cgroupDir, err)
	}

	err = writeNumberToFile(cgroupDir+"/cgroup.procs", os.Getpid(), 0700)
	if err != nil {
		return fmt.Errorf("unable to write to cgroup file: %w", err)
	}
	return nil
}

func removeCGroups(containerID string) error {
	cgroups := generateCGroupList(containerID, defaultCGroups)

	for _, cdir := range cgroups {
		if err := os.Remove(cdir); err != nil {
			return fmt.Errorf("unable to remove '%v' directory: %w", cdir, err)
		}
	}
	return nil
}

func setMemLimit(containerID string, limitInMB, swapLimitInMB int) error {
	memFilePath := getCgroupPath("memory", containerID, "memory.limit_in_bytes")
	swpFilePath := getCgroupPath("memory", containerID, "memory.memsw.limit_in_bytes")

	memLimit := limitInMB * 1024 * 1024
	if err := writeMemLimit(memFilePath, memLimit); err != nil {
		return err
	}

	if swapLimitInMB < 0 {
		return nil
	}

	swapLimit := (swapLimitInMB * 1024 * 1024) + memLimit
	if err := writeMemLimit(swpFilePath, swapLimit); err != nil {
		return err
	}

	return nil
}

func setCPULimit(containerID string, limit float64) error {
	cfsPeriodPath := getCgroupPath("cpu", containerID, "cpu.cfs_quota_us")
	cfsQuotaPath := getCgroupPath("cpu", containerID, "cpu.cfs_quota_us")
	if limit > float64(runtime.NumCPU()) {
		return fmt.Errorf("limit '%v' greater than number of available CPUs (%v)", limit, runtime.NumCPU())
	}

	if err := writeNumberToFile(cfsPeriodPath, nsBase, 0644); err != nil {
		return fmt.Errorf("unable to write cfs period: %w", err)
	}

	quota := int(nsBase * limit)
	if err := writeNumberToFile(cfsQuotaPath, quota, 0644); err != nil {
		return fmt.Errorf("unable to write cfs quota: %w", err)
	}
	return nil
}

func setPidLimit(containerID string, limit int) error {
	path := getCgroupPath("pids", containerID, "pids.max")
	if err := writeNumberToFile(path, limit, 0644); err != nil {
		return fmt.Errorf("unable to write pids limit: %v", err)
	}
	return nil
}

func getCgroupPath(cgroup, containerID, name string) string {
	base := fmt.Sprintf(cgroupPathFmt, cgroup, containerID)
	return base + "/" + name
}

func writeMemLimit(path string, limit int) error {
	err := writeNumberToFile(path, limit, 0644)
	if err != nil {
		return fmt.Errorf("unable to write memory limit: %w", err)
	}
	return nil
}

// just hiding the []byte bit to make things easier to read
func writeNumberToFile(path string, num int, perm fs.FileMode) error {
	return ioutil.WriteFile(
		path,
		[]byte(strconv.Itoa(num)),
		perm,
	)
}

func generateCGroupList(containerID string, systems []string) []string {
	var out []string
	for _, sys := range systems {
		out = append(out, fmt.Sprintf(cgroupPathFmt, sys, containerID))
	}
	return out
}

func createDirsIfMissing(dirs []string) error {
	for _, d := range dirs {
		i, err := os.Stat(d)

		if os.IsNotExist(err) {
			// dir doesn't exist, make it!
			if err = os.MkdirAll(d, 0755); err != nil {
				return fmt.Errorf("unable to create '%v' directory: %w", d, err)
			}
			continue
		}

		if !i.IsDir() {
			return fmt.Errorf("'%v' already exists, but is not directory", d)
		}
	}

	return nil
}

////////////
/// old  ///
////////////

// cg sets up cgroups
//
// see https://git.kernel.org/pub/scm/linux/kernel/git/tj/cgroup.git/tree/Documentation/admin-guide/cgroup-v2.rst
// for more details on how to do cgroups v2 properly
func cg(name string) {
	cgroups := "/sys/fs/cgroup/"

	//// PIDS
	pids := filepath.Join(cgroups, "pids", name)
	os.Mkdir(pids, 0755)

	// limit the number of child processes to 10
	ioutil.WriteFile(filepath.Join(pids, "pids.max"), []byte("10"), 0700)

	// ??
	ioutil.WriteFile(filepath.Join(pids, "notify_on_release"), []byte("1"), 0700)

	//// CPU
	cpu := filepath.Join(cgroups, "cpu", name)
	os.Mkdir(cpu, 0755)

	// set total available run-time within a period
	ioutil.WriteFile(filepath.Join(cpu, "cpu.cfs_period_us"), []byte("10000"), 0700)

	// set length of a period
	ioutil.WriteFile(filepath.Join(cpu, "cpu.cfs_quota_us"), []byte("5000"), 0700)

	//// MEMORY
	memory := filepath.Join(cgroups, "memory", name)
	os.Mkdir(memory, 0755)

	// set the memory hard limit
	ioutil.WriteFile(filepath.Join(memory, "memory.limit_in_bytes"), []byte("10000000"), 0700)

	//// FINALIZE
	// write container PID to cgroup.procs for each cgroup type,
	// so that this process and all child processes have these limits
	for _, cg := range []string{pids, cpu, memory} {
		ioutil.WriteFile(
			filepath.Join(cg, "cgroup.procs"),
			[]byte(strconv.Itoa(os.Getpid())),
			0700,
		)
	}
}

// TODO: change url to busybox.tar hosted in this repo ( as a release? )
// TODO: maybe also shorten the url
const niceErrorMsgBase = `
"%s" does not exist.

Please create this directory and unpack a suitable root filesystem inside it.

An example (and small!) rootfs called BusyBox can be found in https://raw.githubusercontent.com/teddyking/ns-process/4.0/assets/busybox.tar

Once you've downloaded it, unpack it with:

 mkdir -p %s
 tar -C %s -xf busybox.tar

`

// errorIfRootFSNotFound ...
func errorIfRootFSNotFound(rootfsPath string) error {
	if _, err := os.Stat(rootfsPath); os.IsNotExist(err) {
		return fmt.Errorf(niceErrorMsgBase, rootfsPath, rootfsPath, rootfsPath)
	}
	return nil
}

func exitIfRootfsNotFound(rootfsPath string) {
	if err := errorIfRootFSNotFound(rootfsPath); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func pivotRoot(newroot string) error {
	putold := filepath.Join(newroot, "/.pivot_root")

	// bind mount newroot to itself - this is a slight hack needed to satisfy the
	// pivot_root requirement that newroot and putold must not be on the same
	// filesystem as the current root
	if err := syscall.Mount(newroot, newroot, "", syscall.MS_BIND|syscall.MS_REC, ""); err != nil {
		return err
	}

	// create putold directory
	if err := os.MkdirAll(putold, 0700); err != nil {
		return err
	}

	// call pivot_root
	if err := syscall.PivotRoot(newroot, putold); err != nil {
		return err
	}

	// ensure current working directory is set to new root
	if err := os.Chdir("/"); err != nil {
		return err
	}

	// umount putold, which now lives at /.pivot_root
	putold = "/.pivot_root"
	if err := syscall.Unmount(putold, syscall.MNT_DETACH); err != nil {
		return err
	}

	// remove putold
	if err := os.RemoveAll(putold); err != nil {
		return err
	}

	return nil
}

func mountProc(newroot string) error {
	source := "proc"
	target := filepath.Join(newroot, "/proc")
	fstype := "proc"
	flags := 0
	data := ""

	os.MkdirAll(target, 0755)
	if err := syscall.Mount(source, target, fstype, uintptr(flags), data); err != nil {
		return err
	}

	return nil
}

func getContainerFSHome(containerID string) string {
	return wrkHomePath + "/" + containerID + "/fs"
}

func runningInContainer() error {
	id := os.Getenv(envJobID)
	rfs := os.Getenv(envRootFSPath)

	syscall.Sethostname([]byte(id))
	cg(fmt.Sprintf("workernator-job-%v", id))
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

// func runCommandInContainer(containerID string, cmd string, args []string) error {
// 	mntPath := getContainerFSHome(containerID)
// }
