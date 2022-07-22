package container

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"
)

// mountProc ...
func (wr *Wrangler) mountProc(containerID string) error {
	newroot := wr.run + "/containers/" + containerID + "/fs/mnt"
	// newroot, err := filepath.Abs(partialRoot)
	// if err != nil {
	// 	return fmt.Errorf("couldn't get absolute path from '%v': %w", partialRoot, err)
	// }
	// newroot := wr.run + "/" + containerID + "/fs/mnt"

	source := "proc"
	target := filepath.Join(newroot, "/proc")
	fstype := "proc"
	flags := 0
	data := ""

	wr.debugLog("attempting to ensure '%v' is present...\n", target)
	if err := os.MkdirAll(target, 0755); err != nil {
		return fmt.Errorf("couldn't make directory: %w", err)
	}

	wr.debugLog("attempting to mount '%v' as '/proc' within container...\n", target)
	if err := syscall.Mount(source, target, fstype, uintptr(flags), data); err != nil {
		return err
	}

	// wr.debugLog("attempting to read /proc/filesystems\n")
	// fs, err := ioutil.ReadFile("/proc/filesystems")
	// if err != nil {
	// 	wr.debugLog("unable to read /proc/filesystems: %v\n", err)
	// } else {
	// 	wr.debugLog("read /proc/filesystems:\n%v\n", string(fs))
	// }

	return nil
}

// pivotRoot ...
func (wr *Wrangler) pivotRoot(containerID string) error {
	newroot := wr.run + "/containers/" + containerID + "/fs/mnt"
	putold := filepath.Join(newroot, "/.pivot_root")

	wr.debugLog("put old: %v\n", putold)

	// bind mount newroot to itself - this is a slight hack needed to satisfy the
	// pivot_root requirement that newroot and putold must not be on the same
	// filesystem as the current root
	wr.debugLog("bind mount newroot to itself: %v\n", newroot)
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

// chrootContainer ...
func (wr *Wrangler) chrootContainer(containerID string) error {
	mntPath := "./" + wr.getContainerFSHome(containerID) + "/mnt"
	wr.debugLog("praring to chroot, mount path: %v\n", mntPath)

	if err := unix.Chroot(mntPath); err != nil {
		return fmt.Errorf("unable set '%v' as chroot: %w", mntPath, err)
	}

	if err := os.Chdir("/"); err != nil {
		return fmt.Errorf("unable to change directory to /: %w", err)
	}

	return nil
}

// mountContainerDirectories ...
func (wr *Wrangler) mountContainerDirectories(containerID string) error {
	wr.debugLog("time to mount container directories\n")
	wr.debugLog("mountContainerDirectories => running as user: euid: %v, uid: %v\n", os.Geteuid(), os.Getuid())

	newRoot := wr.getContainerFSHome(containerID) + "/mnt"

	mounts := []struct {
		source  string
		target  string
		fsType  string
		flags   uint
		options string
	}{
		{source: "proc", target: newRoot + "/proc", fsType: "proc"},
		{source: "sysfs", target: newRoot + "/sys", fsType: "sysfs"},
		// {source: "tmpfs", target: newRoot + "/tmp", fsType: "tempfs"},
		{
			source:  "tmpfs",
			target:  newRoot + "/dev",
			fsType:  "tmpfs",
			flags:   unix.MS_NOSUID | unix.MS_STRICTATIME,
			options: "mode=755",
		},
		{
			source: "devpts",
			target: newRoot + "/dev/pts",
			fsType: "devpts",
		},
	}

	_, err := os.Stat(newRoot + "/tmp")
	if os.IsNotExist(err) {
		mounts = append(mounts, struct {
			source  string
			target  string
			fsType  string
			flags   uint
			options string
		}{source: "tmpfs", target: newRoot + "/tmp", fsType: "tempfs"})
	}

	for _, mnt := range mounts {
		// ensure mount target exists
		// wr.debugLog("mkdirall: %v\n", mnt.target)
		if err := os.MkdirAll(mnt.target, os.ModePerm); err != nil {
			return fmt.Errorf("unable to create target '%v': %w", mnt.target, err)
		}

		// mount
		// wr.debugLog("mount: %v (%v)\n", mnt.source, mnt.fsType)
		flags := uintptr(mnt.flags)
		if err := unix.Mount(mnt.source, mnt.target, mnt.fsType, flags, mnt.options); err != nil {
			return fmt.Errorf("unable to mount '%v' to '%v' (type: %v): %w", mnt.source, mnt.target, mnt.fsType, err)
		}
	}

	for i, name := range []string{"stdin", "stdout", "stderr"} {
		source := "/proc/self/fd/" + strconv.Itoa(i)
		target := newRoot + "/dev/" + name

		wr.debugLog("symlinking '%v': %v to %v\n", name, source, target)
		err := unix.Symlink(source, target)
		if err != nil {
			return fmt.Errorf("unable to symlink %v to %v: %w", source, target, err)
		}
	}

	// wr.debugLog("newroot: %v\n", newRoot)
	wr.debugLog("about to setup special devices\n")
	devices := []struct {
		name  string
		attr  uint32
		major uint32
		minor uint32
	}{
		{name: "null", attr: 0666 | unix.S_IFCHR, major: 1, minor: 3},
		{name: "zero", attr: 0666 | unix.S_IFCHR, major: 1, minor: 3},
		{name: "random", attr: 0666 | unix.S_IFCHR, major: 1, minor: 8},
		{name: "urandom", attr: 0666 | unix.S_IFCHR, major: 1, minor: 9},
		{name: "console", attr: 0666 | unix.S_IFCHR, major: 136, minor: 1},
		{name: "tty", attr: 0666 | unix.S_IFCHR, major: 5, minor: 0},
		{name: "full", attr: 0666 | unix.S_IFCHR, major: 1, minor: 7},
	}

	for _, dev := range devices {
		devName := newRoot + "/dev/" + dev.name

		fd, err := unix.Open(devName, unix.O_CREAT|unix.O_RDWR|unix.O_SYNC, 0666)
		if err != nil {
			wr.debugLog("unable to unix.Open('%v'): %v\n", devName, err)
		}
		dt := int(unix.Mkdev(dev.major, dev.minor))

		// devName := "/dev/" + dev.name
		wr.debugLog("mknod: '%v':", devName)
		if err := unix.Mknodat(fd, dev.name, dev.attr, dt); err != nil {
			// if err := unix.Mknod(devName, dev.attr, dt); err != nil {
			// return fmt.Errorf("unable to mknod: %w (uid: %v gid: %v euid: %v)",
			// 	err, os.Getuid(), os.Getgid(), os.Geteuid())
			wr.debugLog("unable to mknod: %v\n", err)
		} else {
			wr.debugLog("success!\n")
		}
	}

	wr.debugLog("all required directories set up!\n")
	return nil
}

//mountContainerOverlayFS ...
func (wr *Wrangler) mountContainerOverlayFS(ct *Container) error {
	manifestPath := wr.pathForImageManifest(ct.img)
	imagePath := wr.pathToImageDir(ct.img.ShortSHA())

	var m imageManifest
	if err := parseManifest(manifestPath, &m); err != nil {
		return fmt.Errorf("unable to parse image manifest: %w", err)
	}

	var srcLayers []string
	for _, layer := range m[0].Layers {
		srcLayers = append(
			[]string{imagePath + "/" + layer[:12] + "/fs"},
			srcLayers...,
		)
	}

	containerFSHome := wr.getContainerFSHome(ct.id.String())
	mntOptions := "lowerdir=" + strings.Join(srcLayers, ":") +
		",upperdir=" + containerFSHome + "/upper,workdir=" +
		containerFSHome + "/work"
	if err := unix.Mount("none", containerFSHome+"/mnt", "overlay", uintptr(unix.MS_NODEV), mntOptions); err != nil {
		return fmt.Errorf("unable to mount container overlay fs: %w", err)
	}

	if err := unix.Mount("", "/", "", uintptr(unix.MS_PRIVATE|unix.MS_REC), ""); err != nil {
		return fmt.Errorf("unable to remount /: %w", err)
	}

	return nil
}

// createImageTemp ...
func (wr *Wrangler) createImageTemp(img *Image) (string, error) {
	tmpPath := wr.tmp + "/" + img.ShortSHA()
	if err := os.Mkdir(tmpPath, 0755); err != nil {
		return tmpPath, fmt.Errorf("unable to create temporary directory '%v', got error: %w", tmpPath, err)
	}
	return tmpPath, nil
}

// cleanupImageTemp  ...
func (wr *Wrangler) cleanupImageTemp(img *Image) error {
	wr.debugLog("supposed to be cleaning up tmp, not doing that right now!\n")
	tmpPath := wr.tmp + "/" + img.ShortSHA()
	if err := os.RemoveAll(tmpPath); err != nil {
		return err
	}
	return nil
}

// createContainerDirectories ...
func (wr *Wrangler) createContainerDirectories(ct *Container) error {
	baseDir := wr.containerPath(ct.id.String())
	containerDirs := []string{
		baseDir + "/fs",
		baseDir + "/fs/mnt",
		baseDir + "/fs/upper",
		baseDir + "/fs/work",
	}

	for _, dir := range containerDirs {
		if err := mkdirIfNotExist(dir); err != nil {
			return fmt.Errorf("unable to create directory '%v', error: %w", dir, err)
		}
	}

	return nil
}

// umountContainerDirectories  ...
func (wr *Wrangler) umountContainerDirectories(containerID string) error {
	return nil
}

func mkdirIfNotExist(path string) error {
	st, err := os.Stat(path)
	if os.IsNotExist(err) {
		return os.MkdirAll(path, 0755)
	}

	if err != nil {
		return fmt.Errorf("couldn't check if path exists: %w", err)
	}

	if st.IsDir() {
		return nil
	}

	return fmt.Errorf("unable to create directory '%v'", path)
}
