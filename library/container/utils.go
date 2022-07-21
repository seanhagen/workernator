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
func (wr *Wrangler) untar(tarballPath, outputDir string) error {
	var reader io.ReadCloser
	var err error
	hardLinks := map[string]string{}

	wr.debugLog("opening tarball '%v'\n", tarballPath)
	reader, err = os.Open(tarballPath)
	if err != nil {
		wr.debugLog("unable to open tarball: %v\n", err)
		return fmt.Errorf("unable to open tarball: %w", err)
	}
	defer reader.Close()

	wr.debugLog("checking if .tar or .tar.gz:")
	loc := strings.LastIndex(tarballPath, ".tar.gz")
	if loc != -1 && loc == len(tarballPath)-len(".tar.gz") {
		wr.debugLog(" it's .tar.gz!\n")
		// it's a gziped tarball!
		reader, err = gzip.NewReader(reader)
		if err != nil {
			wr.debugLog("unable to create gzip reader: %v\n", err)
			return fmt.Errorf("file '%v' ends in .tar.gz, but can't create gzip reader: %w", tarballPath, err)
		}
	} else {
		wr.debugLog(" it's .tar!\n")
	}

	tarReader := tar.NewReader(reader)

	for {
		wr.debugLog("reading from tar reader!\n")
		header, err := tarReader.Next()
		if errors.Is(err, io.EOF) {
			wr.debugLog("reached end of tarball!\n")
			break
		}
		if err != nil {
			wr.debugLog("unable to read from tarball: %v\n", err)
			return err
		}
		wr.debugLog("next file from tar reader: %v\n", header.Name)

		path := filepath.Join(outputDir, header.Name) // #nosec G305 checking for zipslip below
		// these checks are from the recommended practice
		// https://deepsource.io/gh/friendsofshopware/shopware-cli/issue/GSC-G305/occurrences
		wr.debugLog("path zipslip check: %v\n", path)
		if !strings.HasPrefix(path, filepath.Clean(outputDir)+string(os.PathSeparator)) {
			wr.debugLog("skipping '%v' because of zipslip check 1\n", header.Name)
			continue
		}

		info := header.FileInfo()

		switch header.Typeflag {
		case tar.TypeDir:
			wr.debugLog("'%v' is a directory, creating '%v'\n", header.Name, path)
			if err = os.MkdirAll(path, info.Mode()); err != nil {
				wr.debugLog("unable to create directory: %v\n", err)
				return fmt.Errorf("unable to create directory from tarball: %w", err)
			}

		case tar.TypeLink:
			wr.debugLog("'%v' is a link to %v\n", header.Name, header.Linkname)
			linkFrom := filepath.Join(outputDir, header.Linkname) // #nosec G305 checking for zipslip below
			wr.debugLog("link-from zipslip check: %v\n", linkFrom)
			if !strings.HasPrefix(linkFrom, filepath.Clean(outputDir)+string(os.PathSeparator)) {
				wr.debugLog("skipping '%v' because of zipslip check 2\n", header.Name)
				continue
			}
			wr.debugLog("link-from zipslip check good!\n")

			linkTo := filepath.Join(outputDir, header.Name) // #nosec G305 checking for zipslip below
			wr.debugLog("link-to zipslip check: %v\n", linkTo)
			if !strings.HasPrefix(linkTo, filepath.Clean(outputDir)+string(os.PathSeparator)) {
				wr.debugLog("skipping '%v' because of zipslip check 3\n", header.Name)
				continue
			}
			wr.debugLog("link-to zipslip check good!\n")
			hardLinks[linkTo] = linkFrom
			wr.debugLog("stored hard link for later\n")

		case tar.TypeSymlink:
			wr.debugLog("'%v' is a symlink to '%v'\n", header.Name, header.Linkname)
			linkPath := filepath.Join(outputDir, header.Name) // #nosec G305 checking for zipslip below
			wr.debugLog("link-path zipslip check: %v\n", linkPath)
			if !strings.HasPrefix(linkPath, filepath.Clean(outputDir)+string(os.PathSeparator)) {
				wr.debugLog("skipping '%v' because of zipslip check 4\n", header.Name)
				continue
			}
			wr.debugLog("link-path zipslip check good!\n")

			if err := os.Symlink(header.Linkname, linkPath); err != nil {
				wr.debugLog("error when creating symlink: %v\n", err)
				if os.IsExist(err) {
					continue
				}
				return err
			}
			wr.debugLog("symlink created!\n")

		case tar.TypeReg:
			wr.debugLog("'%v' is a regular file\n", header.Name)
			// ensure any missing directories get created for us
			fileDir := filepath.Dir(path)
			st, err := os.Stat(fileDir)
			if os.IsNotExist(err) {
				wr.debugLog("path to file '%v' doesn't exist, creating\n", fileDir)
				err = os.MkdirAll(fileDir, 0755)
			}
			if err != nil {
				wr.debugLog("unable to create directory '%v' for '%v', error: %v\n", fileDir, header.Name, err)
				return fmt.Errorf("unable to create directory '%v', error: %w", fileDir, err)
			}
			if !st.IsDir() {
				wr.debugLog("path '%v' already exists but is not a directory!\n", fileDir)
				return fmt.Errorf("path '%v' already exists and is not directory", fileDir)
			}

			wr.debugLog("outputting file '%v' to '%v'\n", header.Name, path)
			if err := outputFile(path, header.Name, info.Mode(), tarReader); err != nil {
				wr.debugLog("unable to output file: %v\n", err)
				return err
			}

		default:
			wr.debugLog("untar: file '%v' type unhandled by function: %v\n", header.Name, header.Typeflag)
		}
	}

	wr.debugLog("finished with tarball, now to create hard links\n")

	for k, v := range hardLinks {
		wr.debugLog("creating hard link '%v' -> '%v'\n", k, v)
		if err := os.Link(v, k); err != nil {
			wr.debugLog("unable to create hard link: %v\n", err)
			return fmt.Errorf("unable to link '%v' to '%v', error: %w", k, v, err)
		}
	}

	wr.debugLog("finished processing tarball\n")
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
