package container

import (
	"fmt"
	"os"
	"path/filepath"
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

	return nil
}

// pivotRoot ...
func (wr *Wrangler) pivotRoot(containerID string) error {
	newroot := wr.run + "/containers/" + containerID + "/fs/mnt"
	// partialRoot := wr.run + "/" + containerID + "/fs/mnt"
	// newroot, err := filepath.Abs(partialRoot)
	// if err != nil {
	// 	return fmt.Errorf("couldn't get absolute path from '%v': %w", partialRoot, err)
	// }
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
	if err := unix.Mount("none", containerFSHome+"/mnt", "overlay", 0, mntOptions); err != nil {
		return fmt.Errorf("unable to mount container overlay fs: %w", err)
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

// chrootContainer ...
func (wr *Wrangler) chrootContainer(containerID string) error {
	mntPath := wr.getContainerFSHome(containerID) + "/mnt"
	wr.debugLog("mount path: %v\n", mntPath)

	// if err := unix.Chroot(mntPath); err != nil {
	// 	return fmt.Errorf("unable set '%v' as chroot: %w", mntPath, err)
	// }

	if err := os.Chdir("/"); err != nil {
		return fmt.Errorf("unable to change directory to /: %w", err)
	}

	return nil
}

// mountContainerDirectories ...
func (wr *Wrangler) mountContainerDirectories(containerID string) error {
	wr.debugLog("time to mount container directories\n")
	create := []string{"/proc", "/sys"} //, "/tmp"}
	for _, toCreate := range create {
		wr.debugLog("need to create %v...", toCreate)
		if _, err := os.Stat(toCreate); os.IsNotExist(err) {
			wr.debugLog(" it doesn't exist yet... ")
			if err = os.MkdirAll(toCreate, 0755); err != nil {
				wr.debugLog(" unable to create: %v\n", err)
				return err
			}
			wr.debugLog("created!\n")
		} else {
			wr.debugLog("directory already exists? (%v)\n", err)
		}
	}

	wr.debugLog("mounting /proc file system\n")
	if err := unix.Mount("proc", "/proc", "proc", 0, ""); err != nil {
		return fmt.Errorf("unable to mount proc: %w", err)
	}

	wr.debugLog("mounting /tmp file system\n")
	if err := unix.Mount("tmpfs", "/tmp", "tmpfs", 0, ""); err != nil {
		return fmt.Errorf("unable to mount tmp: %w", err)
	}

	wr.debugLog("mounting /dev file system\n")
	if err := unix.Mount("devtmpfs", "/dev", "tmpfs", 0, ""); err != nil {
		return fmt.Errorf("uanble to mount dev: %w", err)
	}

	// wr.debugLog("creating '/dev/pts' folder\n")
	// if err := os.MkdirAll("/dev/pts", 0755); err != nil {
	// 	return fmt.Errorf("unable to create /dev/pts: %w", err)
	// }

	// wr.debugLog("mounting /dev/pts file system\n")
	// if err := unix.Mount("devpts", "/dev/pts", "devpts", 0, ""); err != nil {
	// 	return fmt.Errorf("unable to mount /dev/pts: %w", err)
	// }

	// wr.debugLog("creating /sys file system\n")
	// if err := unix.Mount("sysfs", "/sys", "sysfs", 0, ""); err != nil {
	// 	return fmt.Errorf("unable to mount /sys: %w", err)
	// }

	wr.debugLog("all required directories set up!\n")
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
