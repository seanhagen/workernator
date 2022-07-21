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
	debug bool

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

	isDev := strings.TrimSpace(os.Getenv("DEV_MODE"))

	wr := &Wrangler{
		debug: isDev != "",
		lib:   conf.LibPath,
		run:   conf.RunPath,
		tmp:   tmp,
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
	// if err := wr.setupVirtualEthOnHost(c); err != nil {
	// 	return nil, err
	// }

	// do the prepare part from 'prepareAndExecuteContainer' now
	if err := wr.finalizePreparation(c); err != nil {
		return nil, err
	}

	return c, nil
}

// finalizePreparation ...
func (wr *Wrangler) finalizePreparation(ct *Container) error {
	errBuf := bytes.NewBuffer(nil)

	cmd := &exec.Cmd{
		Path:   "/proc/self/exe",
		Args:   []string{"/proc/self/exe", wr.commandRoot, setupNetNS, ct.id.String()},
		Stdout: io.Discard,
		Stderr: errBuf,
	}
	if err := cmd.Run(); err != nil {
		wr.debugLog("unable to run network namespace setup command: %v\n", err)
		wr.debugLog("stderr output from network namespace setup command:\n%v\n", errBuf.String())
		return fmt.Errorf("unable to run the network namespace setup command: %w", err)
	}

	errBuf.Truncate(0)

	cmd = &exec.Cmd{
		Path:   "/proc/self/exe",
		Args:   []string{"/proc/self/exe", wr.commandRoot, setupVeth, ct.id.String()},
		Stdout: io.Discard,
		Stderr: errBuf,
	}
	if err := cmd.Run(); err != nil {
		wr.debugLog("unable to run veth setup command: %v\n", err)
		wr.debugLog("stderr output from veth setup command:\n%v\n", errBuf.String())
		return fmt.Errorf("unable to run the veth setup command: %w", err)
	}

	return nil
}

// setupVirtualEthOnHost ...
func (wr *Wrangler) setupVirtualEthOnHost(ct *Container) error {
	veth0 := "veth0_" + ct.id.String()[:6]
	veth1 := "veth1_" + ct.id.String()[:6]

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
	return wr.run + "/containers/" + ct.id.String() + "/fs"
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
	return wr.run + "/containers/" + ct.id.String()
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
		return nil, fmt.Errorf("unable to download image: %w", err)
	}

	return img, nil
}

// downloadImage  ...
func (wr *Wrangler) downloadImage(ctx context.Context, img *Image) error {
	wr.debugLog("creating temporary image location\n")
	tmpPath, err := wr.createImageTemp(img)
	if err != nil {
		wr.debugLog("unable to temporary location '%v' for saving image: %v\n", tmpPath, err)
		return err
	}

	wr.debugLog("saving image to temporary location\n")
	if err := crane.SaveLegacy(img._img, img.Source(), tmpPath+"/"+packageFileName); err != nil {
		wr.debugLog("unable to save image: %v\n", err)
		return fmt.Errorf("unable to save image: %w", err)
	}

	wr.debugLog("extracting data from image tarball\n")
	if err := wr.untarImageTarball(img); err != nil {
		wr.debugLog("unable to extract: %v\n", err)
		return fmt.Errorf("unable to extract image: %w", err)
	}

	wr.debugLog("processing layers from image tarball\n")
	if err := wr.processLayers(img); err != nil {
		wr.debugLog("unable to process image layers: %v\n", err)
		return fmt.Errorf("unable to process image layers: %w", err)
	}

	wr.debugLog("saving distribution info to internal storage: %v:%v -> %v\n", img.dist, img.vers, img.SHA)
	wr.addDistributionVersion(img.dist, img.vers, img.SHA)

	return wr.cleanupImageTemp(img)
}

// cleanupImageTemp  ...
func (wr *Wrangler) cleanupImageTemp(img *Image) error {
	wr.debugLog("supposed to be cleaning up tmp, not doing that right now!\n")
	// tmpPath := wr.tmp + "/" + img.ShortSHA()
	// if err := os.RemoveAll(tmpPath); err != nil {
	// 	return err
	// }
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

// processLayers ...
func (wr *Wrangler) processLayers(img *Image) error {
	tmpPath := wr.tmp + "/" + img.ShortSHA()
	imageManifestPath := tmpPath + "/manifest.json"
	imageConfigPath := tmpPath + "/sha256:" + img.SHA

	wr.debugLog(
		"processing layers\n\ttmp path: %v\n\timage manifest path: %v\n\timage config path: %v\n",
		tmpPath, imageManifestPath, imageConfigPath,
	)

	wr.debugLog("parsing image manifest\n")
	var mani imageManifest
	err := parseManifest(imageManifestPath, &mani)
	if err != nil {
		wr.debugLog("unable to parse manifest '%v', reason: %v\n", imageManifestPath, err)
		return err
	}

	wr.debugLog("handling image layers\n")
	if err := wr.handleImageLayers(tmpPath, img, mani); err != nil {
		wr.debugLog("unable to handle image layers in '%v', reason: %v\n", tmpPath, err)
		return fmt.Errorf("unable to handle image layers: %w", err)
	}

	wr.debugLog("copying manifest '%v' to '%v'\n", imageManifestPath, wr.pathForImageManifest(img))
	if err := wr.copyFile(imageManifestPath, wr.pathForImageManifest(img)); err != nil {
		wr.debugLog("### unable to copy image manifest from '%v' to '%v'\nreason: %v\n",
			imageManifestPath, wr.pathForImageManifest(img), err,
		)
	}

	wr.debugLog("copying config '%v' to '%v'\n", imageConfigPath, wr.configPathForImage(img))
	if err := wr.copyFile(imageConfigPath, wr.configPathForImage(img)); err != nil {
		wr.debugLog("### unable to copy image config from '%v'\n\t\tto '%v'\nreason: %v\n",
			imageConfigPath, wr.configPathForImage(img), err,
		)
	}

	return nil
}

// copyFile ...
func (wr *Wrangler) copyFile(source, destination string) error {
	wr.debugLog("copying file '%v' to '%v'\n", source, destination)
	wr.debugLog("opening source '%v'\n", source)
	in, err := os.Open(source)
	if err != nil {
		wr.debugLog("unable open source: %v\n", err)
		return err
	}
	defer in.Close()

	wr.debugLog("opening destionation: %v\n", destination)
	out, err := os.OpenFile(destination, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
	if err != nil {
		wr.debugLog("unable to open destination: %v\n", err)
		return err
	}
	defer out.Close()

	wr.debugLog("performing copy\n")
	if _, err := io.Copy(out, in); err != nil {
		wr.debugLog("error while copying: %v\n", err)
		return err
	}
	wr.debugLog("successful copy!\n")

	return nil
}

// handleImageLayers ...
func (wr *Wrangler) handleImageLayers(tmpPath string, img *Image, mani imageManifest) error {
	imagesPath := wr.pathToImageDir(img.ShortSHA())
	wr.debugLog("image path: %v\n", imagesPath)
	if err := mkdirIfNotExist(imagesPath); err != nil {
		wr.debugLog("unable to create directory '%v': %v\n", imagesPath, err)
		return err
	}

	wr.debugLog("handling layers\n")
	for _, layer := range mani[0].Layers {
		layerDir := imagesPath + "/" + layer[:12] + "/fs"
		wr.debugLog("layer '%v' directory: %v\n", layer, layerDir)
		if err := mkdirIfNotExist(layerDir); err != nil {
			wr.debugLog("unable to create layer directory: %v\n", err)
			return fmt.Errorf("unable to create layer output directory: %w", err)
		}

		srcLayer := tmpPath + "/" + layer
		wr.debugLog("source layer: %v\nextracting!\n", srcLayer)
		if err := wr.untar(srcLayer, layerDir); err != nil {
			wr.debugLog("unable to extract source layer: %v\n", err)
			return fmt.Errorf("unable to untar layer file '%s': %w", srcLayer, err)
		}
	}
	wr.debugLog("finished handling layers!\n")

	return nil
}

// pathForImageManifest ...
func (wr *Wrangler) pathForImageManifest(img *Image) string {
	return wr.pathToImageDir(img.ShortSHA()) + "/manifest.json"
}

// configPathForImage ...
func (wr *Wrangler) configPathForImage(img *Image) string {
	sha := img.ShortSHA()
	path := wr.pathToImageDir(sha) + "/" + sha + ".json"
	wr.debugLog("\t-- config path for image '%v' -> '%v'\n", img.ShortSHA(), path)
	return path
}

// imagePath  ...
func (wr *Wrangler) pathToImageDir(shortSHA string) string {
	return wr.lib + "/images/" + shortSHA
}

// untarImageTarball  ...
func (wr *Wrangler) untarImageTarball(img *Image) error {
	pathDir := wr.tmp + "/" + img.ShortSHA()
	pathTar := pathDir + "/" + packageFileName
	wr.debugLog("extracting from '%v' into '%v'\n", pathTar, pathDir)

	return wr.untar(pathTar, pathDir)
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
	f, err := os.OpenFile(wr.pathToKnownImageDB(), os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
	if err != nil {
		zap.L().Error(
			"unable to open manifest path",
			zap.String("path", wr.pathToKnownImageDB()),
			zap.Error(err),
		)
		return
	}
	defer f.Close()

	if err := json.NewEncoder(f).Encode(wr.knownImages); err != nil {
		zap.L().Error(
			"unable to encode manifest to output file",
			zap.String("path", wr.pathToKnownImageDB()),
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

// loadOrCreateKnownImageDB  ...
func (wr *Wrangler) loadOrCreateKnownImageDB() error {
	wr.debugLog("checking if known-image db '%v' exists\n", wr.pathToKnownImageDB())
	st, err := os.Stat(wr.pathToKnownImageDB())
	if os.IsNotExist(err) {
		wr.debugLog("db doesn't exist, creating\n")
		return wr.createManifest()
	}
	wr.debugLog("ensuring '%v' isn't a directory...", wr.pathToKnownImageDB())
	if st.IsDir() {
		wr.debugLog("shoot! it's a directory\n")
		return fmt.Errorf("manifest path '%v' points to directory", wr.pathToKnownImageDB())
	}

	wr.debugLog("all good, loading known image db!\n")
	return wr.loadKnownImageData()
}

// loadKnownImageData ...
func (wr *Wrangler) loadKnownImageData() error {
	f, err := os.OpenFile(wr.pathToKnownImageDB(), os.O_RDONLY, 0444)
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

// pathToKnownImageDB ...
func (wr *Wrangler) pathToKnownImageDB() string {
	return wr.lib + "/images.json"
}

// debugLog ...
func (wr *Wrangler) debugLog(line string, args ...any) {
	if !wr.debug {
		return
	}

	_, _ = fmt.Fprintf(os.Stdout, line, args...)
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
	_, _ = rand.Read(hw[2:])
	return hw
}
