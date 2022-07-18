package images

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/moby/moby/api/types/container"
)

const defaultTag = "latest"

type (
	// Image ...
	Image struct {
		name     string
		tags     []string
		shaShort string
		present  bool
	}

	tagShas map[string]string
	imageDB map[string]tagShas

	// this probably exists as a struct elsewhere, but
	imageMeta struct {
		Arch            string           `json:"architecture"`
		Config          container.Config `json:"config"`
		Container       string           `json:"container"`
		ContainerConfig container.Config `json:"container_config"`
		CreatedAt       time.Time        `json:"created"`
		DockerVersion   string           `json:"docker_version"`
		History         any              `json:"history"`
		OS              string           `json:"os"`
		RootFS          any              `json:"root_fs"`
	}
)

/*
{
  },
  "created": "2022-05-23T19:19:31.970967174Z",
  "docker_version": "20.10.12",
  "history": [
    {
      "created": "2022-05-23T19:19:30.413290187Z",
      "created_by": "/bin/sh -c #(nop) ADD file:8e81116368669ed3dd361bc898d61bff249f524139a239fdaf3ec46869a39921 in / "
    },
    {
      "created": "2022-05-23T19:19:31.970967174Z",
      "created_by": "/bin/sh -c #(nop)  CMD [\"/bin/sh\"]",
      "empty_layer": true
    }
  ],
  "os": "linux",
  "rootfs": {
    "type": "layers",
    "diff_ids": [
      "sha256:24302eb7d9085da80f016e7e4ae55417e412fb7e0a8021e95e3b60c67cde557d"
    ]
  }
}
*/
// Present ...
func (i *Image) Present() bool {
	return false
}

// Name  ...
func (i *Image) Name() string {
	return i.name
}

// SHA ...
func (i *Image) SHA() string {
	return ""
}

// ShortSHA ...
func (i *Image) ShortSHA() string {
	return ""
}

// Tags ...
func (i *Image) Tags() []string {
	return nil
}

// GetLocalImage ...
func GetLocalImage(path, name string) (*Image, error) {
	img := &Image{
		name: name,
	}

	return img, nil
}

// DownloadImage ...
func DownloadImage(outputPath, tmpPath, srcName string) (*Image, error) {
	if err := createDirIfNotExists(outputPath); err != nil {
		return nil, err
	}

	if err := createDirIfNotExists(tmpPath); err != nil {
		return nil, err
	}

	// does the image already exist?
	//  -- srcName ->  image name & tag name, check manifest if we already have it

	return nil, fmt.Errorf("not yet")
}

func imageExists(outputPath, srcName string) (*Image, error) {
	// is our source name valid?
	name, tag, err := getImageNameAndTag(srcName)
	if err != nil {
		return nil, err
	}

	// can we load the db?
	db, err := parseMetadata(outputPath)
	if err != nil {
		return nil, fmt.Errorf("unable to parse metadata: %w", err)
	}

	// at this point, we shouldn't run into anything where we'd want to
	// return an error instead of a partially filled out *Image
	img := &Image{
		name: name,
		tags: []string{tag},
	}

	tags, ok := db[name]
	if !ok {
		return img, nil
	}

	sha, ok := tags[tag]
	if !ok {
		return img, nil
	}

	img.shaShort = sha

	return img, nil
}

func parseImageMetadata(path string) (imageMeta, error) {

}

// the 'database' of local images is stored at path + '/db.json'.
// what's returned is a map of maps, { 'image name': { 'image tag': 'image sha'}}
func parseMetadata(path string) (imageDB, error) {
	db := imageDB{}

	path = path + "/db.json"
	isPresent, isDir := fileExists(path)
	if isDir {
		return nil, fmt.Errorf("database file '%v' is a directory", path)
	}
	if !isPresent {
		// if the file isn't present we just return an 'empty' db
		return db, nil
	}

	dbFile, err := os.OpenFile(path, os.O_RDONLY, 0444)
	if err != nil {
		return nil, err
	}

	err = json.NewDecoder(dbFile).Decode(&db)
	return db, err
}

// fileExists ...
func fileExists(path string) (isPresent, isDir bool) {
	stat, err := os.Stat(path)
	isPresent = !os.IsNotExist(err)
	isDir = stat.IsDir()
	return isPresent, isDir
}

// createDirIfNotExists ...
func createDirIfNotExists(path string) error {
	stat, err := os.Stat(path)
	if !os.IsNotExist(err) && stat.IsDir() {
		return nil
	}

	if !stat.IsDir() {
		return fmt.Errorf("'%v' already exists, but is not a directory")
	}

	if err := os.MkdirAll(path, 0755); err != nil {
		return err
	}
	return nil
}
