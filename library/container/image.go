package container

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/google/go-containerregistry/pkg/crane"
	"go.uber.org/zap"
)

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

// downloadImageManifest  ...
func (wr *Wrangler) downloadImageManifest(ctx context.Context, dist, vers string) (*Image, error) {
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

// loadOrCreateKnownImageDB  ...
func (wr *Wrangler) loadOrCreateKnownImageDB() error {
	//wr.debugLog("checking if known-image db '%v' exists\n", wr.pathToKnownImageDB())
	st, err := os.Stat(wr.pathToKnownImageDB())
	if os.IsNotExist(err) {
		wr.debugLog("db doesn't exist, creating\n")
		return wr.createManifest()
	}
	//wr.debugLog("ensuring '%v' isn't a directory...", wr.pathToKnownImageDB())
	if st.IsDir() {
		wr.debugLog("shoot! it's a directory\n")
		return fmt.Errorf("manifest path '%v' points to directory", wr.pathToKnownImageDB())
	}

	//wr.debugLog("all good, loading known image db!\n")
	return wr.loadKnownImageData()
}

// syncKnownImageDBToFile ...
func (wr *Wrangler) syncKnownImageDBToFile() {
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

// createManifest  ...
func (wr *Wrangler) createManifest() error {
	wr.knownImages = map[string]map[string]string{}
	// could set up a goroutine that syncs the manifest to the file as
	// we download more images, but not doing that right now
	return nil
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
