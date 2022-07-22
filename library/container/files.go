package container

import (
	"io"
	"os"
)

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
