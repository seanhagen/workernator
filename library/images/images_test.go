package images

import (
	"fmt"
	"io"
	"io/ioutil"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

/*
   Need to do the following during library initialization:
    - set up the required folders:
       - /var/lib/workernator[/tmp, /images]
       - /var/run/workernator[/containers, /net-ns]

   But don't need to do that here. Images should ask where they should get put, rather than assuming.

   ///////////////////////
   // Container Images! //
   ///////////////////////

   - [x] can turn an image identifier like 'alpine' or 'alpine:3' into an image name and tag,
     providing the 'latest' tag when no tag is specified
   - [ ] can lookup and parse manifest file
   - [ ] can check if an image already exists on disk
   - [ ] can download a new image when asked

*/

func TestLibrary_Image_LoadManifest(t *testing.T) {
	rootPath := "./testdata"
	sha := "e66264b98777"

	var manifest io.ReadCloser
	var err error

	manifest, err = LoadManifest(rootPath, sha)
	require.NoError(t, err)
	require.NotNil(t, manifest)

	t.Cleanup(func() {
		_ = manifest.Close()
	})

	expect := `[
  {
    "Config": "e66264b98777e12192600bf9b4d663655c98a090072e1bab49e233d7531d1294.json",
    "RepoTags": ["alpine"],
    "Layers": [
      "4a973e6cf97f9b1df4635cea58d123fb66af25721859ae3ddab4c68ef3c7e986/layer.tar"
    ]
  }
]
`

	got, err := ioutil.ReadAll(manifest)
	require.NoError(t, err)

	assert.Equal(t, expect, string(got))
}

func TestLibrary_Images_ParseManifest(t *testing.T) {
	rootPath := "./testdata"
	sha := "e66264b98777"

	file, err := LoadManifest(rootPath, sha)
	require.NoError(t, err)
	require.NotNil(t, file)

	var manifest ImageManifest

	manifest, err = ParseImagesManifest(file)
	require.NotNil(t, manifest)
	require.NoError(t, err)
	require.Len(t, manifest, 1, "only expected one item in manifest")

	got := manifest[0]
	expectConfig := "e66264b98777e12192600bf9b4d663655c98a090072e1bab49e233d7531d1294.json"
	expectTags := []string{"alpine"}
	expectLayers := []string{"4a973e6cf97f9b1df4635cea58d123fb66af25721859ae3ddab4c68ef3c7e986/layer.tar"}

	assert.Equal(t, expectConfig, got.Config)
	assert.Equal(t, expectTags, got.Tags)
	assert.Equal(t, expectLayers, got.Layers)
}

func TestLibrary_Images_GetImageNameAndTag(t *testing.T) {
	tests := []struct {
		src         string
		expectImage string
		expectTag   string
		valid       bool
	}{
		{"alpine", "alpine", "latest", true},
		{"alpine:1", "alpine", "1", true},
		{"viz:2.3.4", "viz", "2.3.4", true},
		{"", "", "", false},
	}

	for i, tt := range tests {
		t.Run(fmt.Sprintf("test_%v", i), func(t *testing.T) {
			var imgName string
			var tagName string
			var err error

			imgName, tagName, err = getImageNameAndTag(tt.src)
			assert.Equal(t, tt.expectImage, imgName)
			assert.Equal(t, tt.expectTag, tagName)

			if tt.valid {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}
		})
	}
}

func TestLibrary_Images_DownloadIfMissingSteps(t *testing.T) {
	require := require.New(t)

	imagesPath := "./testdata/images"
	tmpPath := "./testdata/images/tmp"

	// first image is an alpine image that doesn't exist on the system yet
	imageNameOne := "alpine:3.16"
	// second image also shouldn't exist on the system yet
	imageNameTwo := "alpine:3.15"
	// shouldn't need to be downloaded, as it should be identified as the same
	// image as 'alpine:3.16'
	imageNameThree := "alpine:3.16.0"

	var imageOneSha, imageThreeSha string
	var imageOneTags, imageThreeTags []string

	// nice little helper/container for image data. will have a 'Present() bool' method
	// that returns true if the image already exists locally, and false otherwise
	var image *Image
	var err error

	// checks ./testdata/images/images.json to see if we have already
	// download the tarball for this image. if we have, returns an image
	// struct with all the data required to continue. if the image
	// /doesn't/ exist locally, the struct returned will return false
	// when 'Present()' is called.
	//
	// should only return an error when something weird happens or goes
	// wrong. stuff like the folder where the image belongs is present
	// but either not a directory or not readable or writeable
	image, err = GetLocalImage(imagesPath, imageNameOne)
	require.NoError(err)
	require.NotNil(image)
	require.Equal(imageNameOne, image.Name())
	require.False(image.Present(), "image shouldn't be present yet")

	// now let's download the image and do all the stuff we need to do:
	//   - download tarball to temporary folder
	//   - extract the tarball to the images folder
	//   - copy some stuff
	//   - clean up the temporary folder
	image, err = DownloadImage(imagesPath, tmpPath, imageNameOne)
	require.NotNil(image)
	require.NoError(err)
	require.True(image.Present())
	require.Equal(imageNameOne, image.Name())
	require.NotEmpty(image.SHA())
	require.NotEmpty(image.ShortSHA())

	imageOneSha = image.SHA()
	imageOneTags = image.Tags()
	require.GreaterOrEqual(1, len(imageOneTags))

	imgTmp := tmpPath + "/" + image.ShortSHA()
	require.NoDirExists(imgTmp, "download didn't clean up after itself, '%v' still exists", imgTmp)

	// not downloading this, but checking to ensure it returns what we
	// expect after downloading the first image
	image, err = GetLocalImage(imagesPath, imageNameTwo)
	require.NotNil(image)
	require.NoError(err)
	require.False(image.Present())

	// and now we tell the system to download an image it should already
	// have, just under a different tag
	image, err = DownloadImage(imagesPath, tmpPath, imageNameThree)
	require.NotNil(image)
	require.NoError(err)
	require.True(image.Present())
	require.NotEmpty(image.SHA())
	require.NotEmpty(image.ShortSHA())

	imageThreeSha = image.SHA()
	imageThreeTags = image.Tags()

	// the sha for image 1 and image 3 should be the same
	require.Equal(imageOneSha, imageThreeSha)
	// the tags for image three should contain the tag from image one
	require.Contains(imageThreeTags, imageOneTags[0])
}
