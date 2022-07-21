package container

import (
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

// umountNetworkNamespace ...
func unmountNetworkNamespace(containerID, runDir string) error {
	netNsPath := runDir + "/" + netNsDirName + "/" + containerID
	return unmountDir(netNsPath)
}

// unmountContainerFS ...
func unmountContainerFS(containerID, runDir string) error {
	path := runDir + "/containers/" + containerID + "/fs/mnt"
	return unmountDir(path)
}

// removeContainerCGroups ...
func removeContainerCGroups(containerID string) error {
	path := cgroupBasePath + "/" + containerID
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("unable to remove cgroup path '%v': %w", path, err)
	}
	return nil
}

// unmountDir ...
func unmountDir(dir string) error {
	if err := unix.Unmount(dir, 0); err != nil {
		return fmt.Errorf("unable to umount '%v': %w", dir, err)
	}
	return nil
}
