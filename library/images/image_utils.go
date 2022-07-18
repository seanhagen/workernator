package images

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

type (
	// ImageManifest ...
	ImageManifest []imgManifest

	imgManifest struct {
		Config string   `json:"Config"`
		Tags   []string `json:"RepoTags"`
		Layers []string `json:"Layers"`
	}
)

// LoadManifest ...
func LoadManifest(manifestRoot, sha string) (io.ReadCloser, error) {
	path := manifestRoot + "/" + sha + "/manifest.json"

	st, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("unable to stat '%v', %w", path, err)
	}
	if st.IsDir() {
		return nil, fmt.Errorf("'%v' is a directory, not a file", path)
	}

	return os.OpenFile(path, os.O_RDONLY, 0444)
}

// ParseImagesManifest ...
func ParseImagesManifest(manifest io.Reader) (ImageManifest, error) {
	var out ImageManifest
	err := json.NewDecoder(manifest).Decode(&out)
	return out, err
}

// getImageNameAndTag ...
func getImageNameAndTag(input string) (string, string, error) {
	bits := strings.Split(input, ":")

	var img string
	var tag string = "latest"

	if len(bits) == 0 {
		return "", "", fmt.Errorf("input '%v' not a valid image name and tag, expected format 'image:tag'", input)
	}

	img = strings.TrimSpace(bits[0])
	if img == "" {
		return "", "", fmt.Errorf("input '%v' not a valid image name and tag, expected format 'image:tag'", input)
	}

	if len(bits) > 1 {
		tag = strings.TrimSpace(bits[1])
	}
	if tag == "" {
		tag = defaultTag
	}

	return img, tag, nil
}
