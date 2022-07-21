package container

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"strings"
	"sync"

	"github.com/google/go-containerregistry/pkg/crane"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/rs/xid"
	"github.com/vishvananda/netlink"
	"go.uber.org/zap"
	"golang.org/x/sys/unix"
)

const (
	defaultTag      string = "latest"
	packageFileName string = "package.tar"
)

// Container ...
type Container struct {
	ID  xid.ID
	img *Image
}

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
	lib string
	run string
	tmp string

	commandRoot string

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

	wr := &Wrangler{
		lib: conf.LibPath,
		run: conf.RunPath,
		tmp: tmp,
	}

	if err := wr.loadOrCreateManifest(); err != nil {
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
	img, err := wr.downloadManifest(ctx, dist, vers)
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
		ID:  xid.New(),
		img: img,
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

// LaunchContainer ...
func (wr *Wrangler) LaunchContainer(container *Container) error {
	/*
				var opts []string
				if mem > 0 {
					opts = append(opts, "--mem="+strconv.Itoa(mem))
				}
				if swap >= 0 {
					opts = append(opts, "--swap="+strconv.Itoa(swap))
				}
				if pids > 0 {
					opts = append(opts, "--pids="+strconv.Itoa(pids))
				}
				if cpus > 0 {
					opts = append(opts, "--cpus="+strconv.Itoa(cpus))
				}
				opts = append(opts, "--img="+imageShaHex)
				args := append([]string{containerID}, cmdArgs...)
				args = append(opts, args...)
				args = append([]string{"child-mode"}, args...)
				cmd = exec.Command("/proc/self/exe", args...)
				cmd.Stdin = os.Stdin
				cmd.Stdout = os.Stdout
				cmd.Stderr = os.Stderr
				cmd.SysProcAttr = &unix.SysProcAttr{
					Cloneflags: unix.CLONE_NEWPID |
		      unix.CLONE_NEWUSER |
						unix.CLONE_NEWNS |
						unix.CLONE_NEWUTS |
						unix.CLONE_NEWIPC,
				}
				fmt.Printf("launching command %v for really reals\n", args)
				doOrDie(cmd.Run())
	*/

	// unmount network namespace

	// umount container fs

	// remove cgroups

	// remove container folder
}

// finalizePreparation ...
func (wr *Wrangler) finalizePreparation(ct *Container) error {
	errBuf := bytes.NewBuffer(nil)

	cmd := &exec.Cmd{
		Path:   "/proc/self/exe",
		Args:   []string{"/proc/self/exe", wr.commandRoot, setupNetNS, ct.ID.String()},
		Stdout: io.Discard,
		Stderr: errBuf,
	}
	if err := cmd.Run(); err != nil {
		zap.L().Error("stderr output from setup network namespace command", zap.String("output", errBuf.String()))
		return fmt.Errorf("unable to run the setup network namespace command: %w", err)
	}

	errBuf.Truncate(0)

	cmd = &exec.Cmd{
		Path:   "/proc/self/exe",
		Args:   []string{"/proc/self/exe", wr.commandRoot, setupVeth, ct.ID.String()},
		Stdout: io.Discard,
		Stderr: errBuf,
	}
	if err := cmd.Run(); err != nil {
		zap.L().Error("stderr output from setup veth command", zap.String("output", errBuf.String()))
		return fmt.Errorf("unable to run the setup vethcommand: %w", err)
	}

	return nil
}

// setupVirtualEthOnHost ...
func (wr *Wrangler) setupVirtualEthOnHost(ct *Container) error {
	veth0 := "veth0_" + ct.ID.String()[:6]
	veth1 := "veth1_" + ct.ID.String()[:6]

	linkAttrs := netlink.NewLinkAttrs()
	linkAttrs.Name = veth0

	veth0Struct := &netlink.Veth{
		LinkAttrs:        linkAttrs,
		PeerName:         veth1,
		PeerHardwareAddr: createMACAddress(),
	}
	if err := netlink.LinkAdd(veth0Struct); err != nil {
		return fmt.Errorf("unable to add link: %w", err)
	}

	if err := netlink.LinkSetUp(veth0Struct); err != nil {
		return fmt.Errorf("unable to setup link: %w", err)
	}
	linkBridge, _ := netlink.LinkByName("workernator0")
	if err := netlink.LinkSetMaster(veth0Struct, linkBridge); err != nil {
		return fmt.Errorf("unable to setup link master: %w", err)
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

	containerFSHome := wr.getContainerFSHome(ct)
	mntOptions := "lowerdir=" + strings.Join(srcLayers, ":") +
		",upperdir=" + containerFSHome + "/upper,workdir=" +
		containerFSHome + "/work"
	if err := unix.Mount("none", containerFSHome+"/mnt", "overlay", 0, mntOptions); err != nil {
		return fmt.Errorf("unable to mount container overlay fs: %w", err)
	}

	return nil
}

// getContainerFSHome  ...
func (wr *Wrangler) getContainerFSHome(ct *Container) string {
	return wr.run + "/containers/" + ct.ID.String() + "/fs"
}

// createContainerDirectories ...
func (wr *Wrangler) createContainerDirectories(ct *Container) error {
	baseDir := wr.containerPath(ct)
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

// containerPath ...
func (wr *Wrangler) containerPath(ct *Container) string {
	return wr.run + "/containers/" + ct.ID.String()
}

// downloadImageByDistribution ...
func (wr *Wrangler) downloadImageByDistribution(ctx context.Context, dist, vers string) (*Image, error) {
	// definitely don't have the image, but can we fake it
	name := dist + ":" + vers

	ci, err := crane.Pull(name, crane.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("unable to pull image '%v' manifest: %w", name, err)
	}
	manifest, err := ci.Manifest()
	if err != nil {
		return nil, fmt.Errorf("unable to get image '%v' manifest: %w", name, err)
	}

	img := &Image{
		SHA:    manifest.Config.Digest.Hex,
		dist:   dist,
		vers:   vers,
		exists: false,
		_img:   ci,
	}

	if err := wr.downloadImage(ctx, img); err != nil {
		return nil, err
	}

	return img, nil
}

// downloadImage  ...
func (wr *Wrangler) downloadImage(ctx context.Context, img *Image) error {
	tmpPath, err := wr.createImageTemp(img)
	if err != nil {
		return err
	}

	if err := crane.SaveLegacy(img._img, img.Source(), tmpPath+"/"+packageFileName); err != nil {
		return fmt.Errorf("unable to save image: %w", err)
	}

	if err := wr.untarImageTarball(img); err != nil {
		return fmt.Errorf("unable to extract image: %w", err)
	}

	if err := wr.processLayers(img); err != nil {
		return fmt.Errorf("unable to process image layers: %w", err)
	}

	wr.addDistributionVersion(img.dist, img.vers, img.SHA)

	return wr.cleanupImageTemp(img)
}

// cleanupImageTemp  ...
func (wr *Wrangler) cleanupImageTemp(img *Image) error {
	tmpPath := wr.tmp + "/" + img.ShortSHA()
	if err := os.RemoveAll(tmpPath); err != nil {
		return err
	}
	return nil
}

// createImageTemp ...
func (wr *Wrangler) createImageTemp(img *Image) (string, error) {
	tmpPath := wr.tmp + "/" + img.ShortSHA()
	if err := os.Mkdir(tmpPath, 0755); err != nil {
		return "", fmt.Errorf("unable to create temporary directory '%v', got error: %w", tmpPath, err)
	}
	return tmpPath, nil
}

// processLayers ...
func (wr *Wrangler) processLayers(img *Image) error {
	tmpPath := wr.tmp + "/" + img.ShortSHA()
	imageManifestPath := tmpPath + "/manifest.json"
	imageConfigPath := tmpPath + "/" + img.SHA + ".json"

	var mani imageManifest
	err := parseManifest(imageManifestPath, &mani)
	if err != nil {
		return err
	}

	if err := wr.handleImageLayers(tmpPath, img, mani); err != nil {
		return fmt.Errorf("unable to handle image layers: %w", err)
	}

	if err := wr.copyFile(imageManifestPath, wr.pathForImageManifest(img)); err != nil {
		zap.L().Error(
			"unable to copy image manifest path",
			zap.String("source", imageManifestPath),
			zap.String("target", wr.pathForImageManifest(img)),
			zap.Error(err),
		)
	}

	if err := wr.copyFile(imageConfigPath, wr.configPathForImage(img)); err != nil {
		zap.L().Error(
			"unable to copy image manifest path",
			zap.String("source", imageConfigPath),
			zap.String("target", wr.configPathForImage(img)),
			zap.Error(err),
		)
	}

	return nil
}

// copyFile ...
func (wr *Wrangler) copyFile(source, destination string) error {
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Open(destination)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(in, out); err != nil {
		return err
	}

	return nil
}

// handleImageLayers ...
func (wr *Wrangler) handleImageLayers(tmpPath string, img *Image, mani imageManifest) error {
	imagesPath := wr.pathToImageDir(img.ShortSHA())
	if err := mkdirIfNotExist(imagesPath); err != nil {
		return err
	}

	for _, layer := range mani[0].Layers {
		layerDir := imagesPath + "/" + layer[:12] + "/fs"
		if err := mkdirIfNotExist(layerDir); err != nil {
			return fmt.Errorf("unable to create layer output directory: %w", err)
		}

		srcLayer := tmpPath + "/" + layer
		if err := untar(srcLayer, layerDir); err != nil {
			return fmt.Errorf("unable to untar layer file '%s': %w", srcLayer, err)
		}
	}

	return nil
}

// pathForImageManifest ...
func (wr *Wrangler) pathForImageManifest(img *Image) string {
	return wr.pathToImageDir(img.ShortSHA()) + "/manifest.json"
}

// configPathForImage ...
func (wr *Wrangler) configPathForImage(img *Image) string {
	sha := img.ShortSHA()
	return wr.pathToImageDir(sha) + "/" + sha + ".json"
}

// imagePath  ...
func (wr *Wrangler) pathToImageDir(shortSHA string) string {
	return wr.lib + "/images/" + shortSHA
}

// untarImageTarball  ...
func (wr Wrangler) untarImageTarball(img *Image) error {
	pathDir := wr.tmp + "/" + img.ShortSHA()
	pathTar := pathDir + "/" + packageFileName
	return untar(pathTar, pathDir)
}

// downloadManifest  ...
func (wr *Wrangler) downloadManifest(ctx context.Context, dist, vers string) (*Image, error) {
	name := dist + ":" + vers

	img, err := crane.Pull(name, crane.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("unable to pull image '%v' manifest: %w", name, err)
	}
	manifest, err := img.Manifest()
	if err != nil {
		return nil, fmt.Errorf("unable to get image '%v' manifest: %w", name, err)
	}

	fullSha := manifest.Config.Digest.Hex
	if wr.shaExists(dist, fullSha) {
		return wr.loadImageBySha(dist, vers, fullSha)
	}

	return wr.noImageForSha(dist, vers, fullSha)
}

// loadImageBySha ...
func (wr *Wrangler) loadImageBySha(dist, vers, fullSha string) (*Image, error) {
	img := Image{
		SHA:    fullSha,
		dist:   dist,
		vers:   vers,
		exists: true,
	}

	return &img, nil
}

// noImageForSha ...
func (wr *Wrangler) noImageForSha(dist, vers, fullSha string) (*Image, error) {
	img := Image{
		SHA:  fullSha,
		dist: dist,
		vers: vers,
	}

	return &img, nil
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

	wr.syncManifestToFile()
}

// syncManifestToFile ...
func (wr *Wrangler) syncManifestToFile() {
	f, err := os.OpenFile(wr.manifestPath(), os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
	if err != nil {
		zap.L().Error(
			"unable to open manifest path",
			zap.String("path", wr.manifestPath()),
			zap.Error(err),
		)
		return
	}
	defer f.Close()

	if err := json.NewEncoder(f).Encode(wr.knownImages); err != nil {
		zap.L().Error(
			"unable to encode manifest to output file",
			zap.String("path", wr.manifestPath()),
			zap.Error(err),
		)
	}
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

// loadOrCreateManifest  ...
func (wr *Wrangler) loadOrCreateManifest() error {
	st, err := os.Stat(wr.manifestPath())
	if os.IsNotExist(err) {
		return wr.createManifest()
	}

	if os.IsExist(err) {
		return wr.loadManifest()
	}

	if st.IsDir() {
		return fmt.Errorf("manifest path '%v' points to directory", wr.manifestPath())
	}

	return fmt.Errorf("unable to load or create manifest: %w", err)
}

// loadManifest ...
func (wr *Wrangler) loadManifest() error {
	f, err := os.OpenFile(wr.manifestPath(), os.O_RDONLY, 0444)
	if err != nil {
		return fmt.Errorf("unable to open manifest: %w", err)
	}

	if err := json.NewDecoder(f).Decode(&wr.knownImages); err != nil {
		return fmt.Errorf("unable to decode manafest: %w", err)
	}

	return nil
}

// createManifest  ...
func (wr *Wrangler) createManifest() error {
	wr.knownImages = map[string]map[string]string{}
	// could set up a goroutine that syncs the manifest to the file as
	// we download more images, but not doing that right now
	return nil
}

// manifestPath ...
func (wr *Wrangler) manifestPath() string {
	return wr.lib + "/images.json"
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

func distAndVersionFromSource(source string) (string, string, error) {
	if !strings.Contains(source, ":") {
		return source, defaultTag, nil
	}

	parts := strings.Split(source, ":")
	if len(parts) < 2 {
		return "", "", fmt.Errorf("source missing tag, got '%v'; expected string after ':'", source)
	}

	return parts[0], parts[1], nil
}

func parseManifest(path string, m *imageManifest) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("unable to open manifest: %w", err)
	}

	if err := json.NewDecoder(f).Decode(&m); err != nil {
		return fmt.Errorf("unable to decode manifest: %w", err)
	}
	if len(*m) == 0 || len((*m)[0].Layers) == 0 {
		return fmt.Errorf("no layers in image manifest")
	}
	if len(*m) > 1 {
		return fmt.Errorf("cannot currently handle multi-manifest images")
	}

	return nil
}

func createMACAddress() net.HardwareAddr {
	hw := make(net.HardwareAddr, 6)
	hw[0] = 0x03
	hw[1] = 0x43
	rand.Read(hw[2:])
	return hw
}
