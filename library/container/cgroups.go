package container

import (
	"bufio"
	"fmt"
	"io/ioutil"
	"os"
	"strconv"
	"strings"

	"github.com/davecgh/go-spew/spew"
)

const cgroupBasePath string = "/sys/fs/cgroup/workernator"

// const defaultMemory int = 10 * 1024 * 1024 // 10 mb
// const defaultCPUWeight int = 50
// const defaultPidLimit int = 10 // max 10 pids within container

// setupContainerCgroups ...
func (wr *Wrangler) setupContainerCgroups(containerID string) error {
	// create the cgroups
	if err := wr.createCGroups(containerID); err != nil {
		return err
	}
	// configure cgroups
	return wr.addContainerToCGroups(containerID)
}

// addContainerToCGroups ...
func (wr *Wrangler) addContainerToCGroups(containerID string) error {
	path := wr.getCgroupFile(containerID, "cgroup.procs")
	err := ioutil.WriteFile(
		path,
		[]byte(strconv.Itoa(os.Getpid())),
		0700,
	)
	if err != nil {
		return fmt.Errorf("uanble to write pid '%v' to '%v': %w ", os.Getpid(), path, err)
	}

	return nil
}

// createCGroups ...
func (wr *Wrangler) createCGroups(containerID string) error {
	if err := mkdirIfNotExist(cgroupBasePath); err != nil {
		return fmt.Errorf("unable to create base cgroup folder '%v': %w", cgroupBasePath, err)
	}

	if err := wr.setupCgroupSubtreeControl(); err != nil {
		return err
	}

	containerCgroupPath := cgroupBasePath + "/" + containerID
	wr.debugLog("creating container cgroup path: %v\n", containerCgroupPath)
	if err := mkdirIfNotExist(containerCgroupPath); err != nil {
		return fmt.Errorf("unable to create container cgroup folder '%v': %w", containerCgroupPath, err)
	}

	return nil
}

// setContainerPidLimit ...
func (wr *Wrangler) setContainerPidLimit(containerID string, limit int) error {
	path := wr.getCgroupFile(containerID, "pids.max")
	if err := ioutil.WriteFile(path, []byte(strconv.Itoa(limit)), 0644); err != nil {
		return fmt.Errorf("unable to write '%v' to set pid limit: %w", path, err)
	}

	return nil
}

// setupConatinerIOLimits ...
func (wr *Wrangler) setupContainerIOLimits(containerID string, bps, iops int) error {
	mounts, err := wr.findCgroupMount(containerID)
	if err != nil {
		return err
	}

	ioMaxPath := "/sys/fs/cgroup/workernator/" + containerID + "/io.max"

	for _, m := range mounts {
		toWrite := fmt.Sprintf(
			"%v rbps=%v wbps=%v riops=%v wiops=%v",
			m, bps, bps, iops, iops,
		)
		if err := ioutil.WriteFile(ioMaxPath, []byte(toWrite), 0644); err != nil {
			return fmt.Errorf("unable to write '%v' to set max io (bps: %v, iops: %v): %w", ioMaxPath, bps, iops, err)
		}
	}

	return nil
}

// findCgroupMount ...
func (wr *Wrangler) findCgroupMount(containerID string) ([]string, error) {
	// f, err := os.Open("/proc/self/mountinfo")
	f, err := os.Open("/proc/partitions")
	if err != nil {
		return nil, fmt.Errorf("unable to open '/proc/self/mountinfo': %w", err)
	}

	// expectMount := wr.getContainerFSHome(containerID)

	var devs []string

	wr.debugLog("scanning mountinfo...\n")
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		txt := scanner.Text()
		fields := strings.Split(txt, " ")
		if len(fields) != 4 {
			continue
		}
		spew.Dump(fields)

		wr.debugLog("device: %v %v -> %v\n", fields[0], fields[1], fields[3])

		devs = append(devs, fields[0]+":"+fields[1])
	}
	return devs, nil
	//return nil, fmt.Errorf("could not find cgroup mount for container '%v'", containerID)
}

// setupContainerMemoryLimits ...
func (wr *Wrangler) setupContainerMemoryLimits(containerID string, limitMB int) error {
	memHighPath := "/sys/fs/cgroup/workernator/" + containerID + "/memory.high"
	memMaxPath := "/sys/fs/cgroup/workernator/" + containerID + "/memory.max"

	var highMB int = limitMB / 2

	high := strconv.Itoa(highMB) + "M"
	max := strconv.Itoa(limitMB) + "M"

	if err := ioutil.WriteFile(memHighPath, []byte(high), 0644); err != nil {
		return fmt.Errorf("unable to write memory limit file '%v': %w", memHighPath, err)
	}

	if err := ioutil.WriteFile(memMaxPath, []byte(max), 0644); err != nil {
		return fmt.Errorf("uanble to write memory max file '%v': %w", memMaxPath, err)
	}

	return nil
}

// setupContainerCPUMax  ...
func (wr *Wrangler) setupContainerCPUWeight(containerID string, weight int) error {
	if weight < 1 || weight > 10_000 {
		return fmt.Errorf("cpu weight value must be >= 1 and <= 10,000, got: %v", weight)
	}
	cpuWeightPath := "/sys/fs/cgroup/workernator/" + containerID + "/cpu.weight"
	if err := ioutil.WriteFile(cpuWeightPath, []byte(strconv.Itoa(weight)), 0644); err != nil {
		return fmt.Errorf("unable to write '%v' with cpu weight: %w", cpuWeightPath, err)
	}
	return nil
}

// setupContainerCPULimits ...
func (wr *Wrangler) setupContainerCPUBandwidth(containerID string, max, period int) error {

	if max < period {
		return fmt.Errorf("max must be larger than period, got max: %v, period: %v", max, period)
	}

	cpuMaxPath := "/sys/fs/cgroup/workernator/" + containerID + "/cpu.max"

	if err := ioutil.WriteFile(cpuMaxPath, []byte("200000 100000"), 0644); err != nil {
		return fmt.Errorf("unable to write '%v' with cpu bandwidth limit: %w", cpuMaxPath, err)
	}

	return nil
}

// getCgroupFile  ...
func (wr *Wrangler) getCgroupFile(containerID, file string) string {
	return cgroupBasePath + "/" + containerID + "/" + file
}

// setupCgroupSubtreeControl ...
func (wr *Wrangler) setupCgroupSubtreeControl() error {
	path := cgroupBasePath + "/cgroup.subtree_control"
	write := "+cpu +memory +io +pids"
	if err := ioutil.WriteFile(path, []byte(write), 0644); err != nil {
		return fmt.Errorf("unable to write subtree control file '%v': %w", path, err)
	}
	return nil
}
