package container

const netNsDirName = "net-ns"

// pathToKnownImageDB ...
func (wr *Wrangler) pathToKnownImageDB() string {
	return wr.lib + "/images.json"
}

// getContainerFSHome  ...
func (wr *Wrangler) getContainerFSHome(id string) string {
	return wr.run + "/containers/" + id + "/fs"
}

// containerPath ...
func (wr *Wrangler) containerPath(id string) string {
	return wr.run + "/containers/" + id
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

// pathToNetNs ...
func (wr *Wrangler) pathToNetNs() string {
	return wr.run + "/" + netNsDirName
}

// pathToNSMount ...
func (wr *Wrangler) pathToNSMount() string {
	return wr.pathToNetNs() + "/" + wr.containerID
}
