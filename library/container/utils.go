package container

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"go.uber.org/zap"
)

// untar ...
func untar(tarballPath, outputDir string) error {
	var reader io.ReadCloser
	var err error
	hardLinks := map[string]string{}

	reader, err = os.Open(tarballPath)
	if err != nil {
		return fmt.Errorf("unable to open tarball: %w", err)
	}
	defer reader.Close()

	loc := strings.LastIndex(tarballPath, ".tar.gz")
	if loc != -1 && loc == len(tarballPath)-len(".tar.gz") {
		// it's a gziped tarball!
		reader, err = gzip.NewReader(reader)
		if err != nil {
			return fmt.Errorf("file '%v' ends in .tar.gz, but can't create gzip reader: %w", tarballPath, err)
		}
	}

	tarReader := tar.NewReader(reader)

	for {
		header, err := tarReader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}

		path := filepath.Join(outputDir, header.Name)
		info := header.FileInfo()

		switch header.Typeflag {
		case tar.TypeDir:
			if err = os.MkdirAll(path, info.Mode()); err != nil {
				return fmt.Errorf("unable to create directory from tarball: %w", err)
			}

		case tar.TypeLink:
			linkFrom := filepath.Join(outputDir, header.Linkname)
			linkTo := filepath.Join(outputDir, header.Name)
			hardLinks[linkTo] = linkFrom

		case tar.TypeSymlink:
			linkPath := filepath.Join(outputDir, header.Name)
			if err := os.Symlink(header.Linkname, linkPath); err != nil {
				if os.IsExist(err) {
					continue
				}
				return err
			}
		case tar.TypeReg:
			// ensure any missing directories get created for us
			fileDir := filepath.Dir(path)
			st, err := os.Stat(fileDir)
			if os.IsNotExist(err) {
				err = os.MkdirAll(fileDir, 0755)
			}
			if err != nil {
				return fmt.Errorf("unable to create directory '%v', error: %w", fileDir, err)
			}
			if !st.IsDir() {
				return fmt.Errorf("path '%v' already exists and is not directory", fileDir)
			}

			if err := outputFile(path, header.Name, info.Mode(), tarReader); err != nil {
				return err
			}

		default:
			zap.L().Debug("untar: file type unhandled by untar function", zap.ByteString("type", []byte{header.Typeflag}))
		}
	}

	for k, v := range hardLinks {
		if err := os.Link(v, k); err != nil {
			return fmt.Errorf("unable to link '%v' to '%v', error: %w", k, v, err)
		}
	}
	return nil
}

func outputFile(path, name string, mode fs.FileMode, reader io.Reader) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	defer func() {
		if file != nil {
			if err := file.Close(); err != nil {
				zap.L().Error("unable to close file", zap.String("file", path), zap.Error(err))
			}
		}
	}()

	if os.IsExist(err) {
		return nil
	}

	if err != nil {
		return fmt.Errorf("unable to open output '%v' for tarball file '%v', error: %w", path, name, err)
	}

	_, err = io.Copy(file, reader)
	return err
}
