package container

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/rs/xid"
	"github.com/vishvananda/netlink"
)

const (
	defaultTag      string = "latest"
	packageFileName string = "package.tar"
	devMode         string = "DEV_MODE"
)

// Image ...
type Image struct {
	SHA string

	dist, vers string
	exists     bool
	_img       v1.Image
}

// Source  ...
func (i *Image) Source() string {
	return i.dist + ":" + i.vers
}

// Distribution ...
func (i *Image) Distribution() string {
	return i.dist
}

// Version  ...
func (i *Image) Version() string {
	return i.vers
}

// ShortSHA ...
func (i *Image) ShortSHA() string {
	if len(i.SHA) < 12 {
		return i.SHA
	}
	return i.SHA[:12]
}

// Config ...
type Config struct {
	LibPath string
	RunPath string
	TmpPath string
}

// Wrangler ...
type Wrangler struct {
	debug       bool
	commandRoot string

	lib string
	run string
	tmp string

	processingConntainer bool
	containerID          string

	knownImagesLock sync.RWMutex
	knownImages     map[string]map[string]string
}

type imageManifest []struct {
	Config   string
	RepoTags []string
	Layers   []string
}

// type imageConfigDetails struct {
// 	Env []string `json:"Env"`
// 	Cmd []string `json:"Cmd"`
// }
// type imageConfig struct {
// 	Config imageConfigDetails `json:"config"`
// }

// NewWrangler ...
func NewWrangler(conf Config) (*Wrangler, error) {
	if err := mkdirIfNotExist(conf.LibPath); err != nil {
		return nil, fmt.Errorf("unable to create lib directory: %w", err)
	}

	if err := mkdirIfNotExist(conf.RunPath); err != nil {
		return nil, fmt.Errorf("unable to create run directory: %w", err)
	}

	tmp, err := os.MkdirTemp(conf.TmpPath, "workernator-wrangler")
	if err != nil {
		return nil, fmt.Errorf("unable to create temporary directory: %w", err)
	}

	isDev := strings.TrimSpace(os.Getenv(devMode))
	if isDev == "" {
		_, _ = fmt.Fprintf(os.Stdout, "we are not in dev mode!\n")
	}
	wr := &Wrangler{
		debug: isDev != "",
		lib:   conf.LibPath,
		run:   conf.RunPath,
		tmp:   tmp,
	}

	links, err := netlink.LinkList()
	if err != nil {
		return nil, fmt.Errorf("unable to get list of net links: %w", err)
	}

	if !wr.isBridgeUp(links) {
		if err := wr.setupBridge(); err != nil {
			return nil, err
		}
	}

	if err := wr.loadOrCreateKnownImageDB(); err != nil {
		return nil, err
	}

	return wr, nil
}

// GetImage ...
func (wr *Wrangler) GetImage(ctx context.Context, source string) (*Image, error) {
	dist, vers, err := distAndVersionFromSource(source)
	if err != nil {
		return nil, err
	}

	versions, ok := wr.knownImages[dist]
	if !ok {
		// the image dist isn't known, definitely have to download
		return wr.downloadImageByDistribution(ctx, dist, vers)
	}

	sha, ok := versions[vers]
	if ok {
		// looks like we've already downloaded this image before
		return wr.loadImageBySha(dist, vers, sha)
	}

	// download image manifest, check if sha matches image we already have
	img, err := wr.downloadImageManifest(ctx, dist, vers)
	if err != nil {
		return nil, err
	}

	// if this image alredy exists, nothing else to to do here!
	if img.exists {
		return img, nil
	}

	// doesn't exist, NOW we finally download the image
	if err := wr.downloadImage(ctx, img); err != nil {
		return nil, err
	}
	return img, nil
}

// PrepImageForLaunch ...
func (wr *Wrangler) PrepImageForLaunch(img *Image) (*Container, error) {
	c := &Container{
		id:          xid.New(),
		img:         img,
		baseCommand: wr.commandRoot,
	}

	// create container directories
	if err := wr.createContainerDirectories(c); err != nil {
		return nil, fmt.Errorf("unable to create container directories: %w", err)
	}

	// mount overlay file system
	if err := wr.mountContainerOverlayFS(c); err != nil {
		return nil, err
	}

	// setup virtual eth on host
	if err := wr.setupVirtualEthOnHost(c); err != nil {
		return nil, err
	}

	// do the prepare part from 'prepareAndExecuteContainer' now
	if err := wr.finalizePreparation(c); err != nil {
		return nil, err
	}

	return c, nil
}

// finalizePreparation ...
func (wr *Wrangler) finalizePreparation(ct *Container) error {
	errBuf := bytes.NewBuffer(nil)

	baseArgs := []string{
		"/proc/self/exe",
		wr.commandRoot,
	}

	postArgs := []string{
		wr.lib,
		wr.run,
		wr.tmp,
		ct.id.String(),
	}

	env := os.Environ()
	if wr.debug {
		env = append(env, devMode+"=on")
	}

	cmd := &exec.Cmd{
		Path: "/proc/self/exe",
		//Args: []string{"/proc/self/exe", wr.commandRoot, setupNetNS, ct.id.String()},
		Args: append(baseArgs, append([]string{setupNetNS}, postArgs...)...),
		Env:  env,
		// Stdout: io.Discard,
		Stdout: os.Stdout,
		Stderr: errBuf,
	}
	if err := cmd.Run(); err != nil {
		wr.debugLog("unable to run network namespace setup command: %v\n", err)
		wr.debugLog("stderr output from network namespace setup command:\n%v\n", errBuf.String())
		return fmt.Errorf("unable to run the network namespace setup command: %w", err)
	}

	errBuf.Truncate(0)

	cmd = &exec.Cmd{
		Path: "/proc/self/exe",
		//Args: []string{"/proc/self/exe", wr.commandRoot, setupVeth, ct.id.String()},
		Args: append(baseArgs, append([]string{setupVeth}, postArgs...)...),
		Env:  env,
		// Stdout: io.Discard,
		Stdout: os.Stdout,
		Stderr: errBuf,
	}
	if err := cmd.Run(); err != nil {
		wr.debugLog("unable to run veth setup command: %v\n", err)
		wr.debugLog("stderr output from veth setup command:\n%v\n", errBuf.String())
		return fmt.Errorf("unable to run the veth setup command: %w", err)
	}

	ct.pathToContainerFs = wr.containerPath(ct.id.String())
	ct.pathToRunDir = wr.run

	return nil
}

// addDistributionVersion ...
func (wr *Wrangler) addDistributionVersion(dist, vers, sha string) {
	wr.knownImagesLock.Lock()
	defer wr.knownImagesLock.Unlock()

	versions, ok := wr.knownImages[dist]
	if !ok {
		versions = map[string]string{}
	}

	versions[vers] = sha
	wr.knownImages[dist] = versions

	wr.syncKnownImageDBToFile()
}

// shaExists ...
func (wr *Wrangler) shaExists(dist, sha string) bool {
	wr.knownImagesLock.RLock()
	versions, ok := wr.knownImages[dist]
	wr.knownImagesLock.RUnlock()

	if !ok {
		// don't have any versions for this distribution
		return false
	}

	for _, fullSha := range versions {
		if fullSha == sha {
			return true
		}
	}

	return false
}

// debugLog ...
func (wr *Wrangler) debugLog(line string, args ...any) {
	if !wr.debug {
		return
	}

	_, _ = fmt.Fprintf(os.Stdout, line, args...)
}
