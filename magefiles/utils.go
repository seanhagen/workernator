//go:build mage
// +build mage

package main

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/magefile/mage/target"
)

func fileExists(path string) (bool, error) {
	st, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	if st.IsDir() {
		return false, fmt.Errorf("path is a directory, not a file")
	}

	mode := st.Mode()
	if !mode.Perm().IsRegular() {
		return false, fmt.Errorf("path is not a regular file?")
	}

	return true, nil
}

func dirExists(path string) (bool, error) {
	st, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return st.IsDir(), nil
}

func extractFilesFromTar(tarball *os.File, files []string, outdir string) error {
	info, err := os.Stat(outdir)
	if errors.Is(err, os.ErrNotExist) {
		if err = os.MkdirAll(outdir, os.ModeDir|0755); err != nil {
			return fmt.Errorf("unable to create output directory: %w", err)
		}
		info, err = os.Stat(outdir)
	}

	if err != nil {
		return fmt.Errorf("unable to get output directory info: %w", err)
	}

	if !info.IsDir() {
		return fmt.Errorf("'%v' exists but is not a directory", outdir)
	}

	// seek back to the start of the tarball, just in case
	_, err = tarball.Seek(0, 0)
	if err != nil {
		return fmt.Errorf("unable to seek: %w", err)
	}

	gz, err := gzip.NewReader(tarball)
	if err != nil {
		return fmt.Errorf("unable to open tarball with gzip: %w", err)
	}

	tb := tar.NewReader(gz)
	for {
		header, err := tb.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("unable to read from tarball: %w", err)
		}

		if header.Typeflag != tar.TypeReg {
			continue
		}

		filename := filepath.Base(header.Name)
		if !fileInList(filename, files) {
			continue
		}

		outputFilePath := filepath.Join(outdir, filename)
		writer, err := os.OpenFile(outputFilePath, os.O_CREATE|os.O_WRONLY, os.FileMode(header.Mode))
		if err != nil {
			return fmt.Errorf("unable to create output file for '%v', %v", outputFilePath, err)
		}
		defer writer.Close()

		_, err = io.Copy(writer, tb)
		if err != nil {
			return fmt.Errorf("unable to copy data from tarball to output: %w", err)
		}
	}

	return nil
}

func fileInList(filename string, data []string) bool {
	for _, v := range data {
		if v == filename {
			return true
		}
	}
	return false
}

func checkNewer(outputs, inputs []string) (bool, error) {
	for _, o := range outputs {
		m, err := target.Path(o, inputs...)
		if !m || err != nil {
			return m, err
		}
	}
	return true, nil
}
