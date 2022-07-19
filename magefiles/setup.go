//go:build mage
// +build mage

package main

import (
	"fmt"
	"os"
)

const buildDir string = "build"

func ensureBuildDir() error {
	fmt.Fprintf(os.Stdout, "[PREP] Checking for build output directory '%v'...", buildDir)
	st, err := os.Stat(buildDir)
	if os.IsNotExist(err) {
		fmt.Fprintf(os.Stdout, " build output directory doesn't exist, creating '%v'...", buildDir)
		if err := os.Mkdir(buildDir, 0755); err != nil {
			fmt.Fprintf(os.Stdout, " ERROR: %v\n", err)
			return err
		}
		fmt.Fprintf(os.Stdout, " SUCCESS\n")
		return nil
	}

	if st.IsDir() {
		fmt.Fprintf(os.Stdout, " build output directory exists, done\n")
		return nil
	}

	return fmt.Errorf("cannot create build directory '%v', a file already exists with that name")
}
