package workernator

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
)

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
