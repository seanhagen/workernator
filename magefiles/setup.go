//go:build mage
// +build mage

package main

import (
	"fmt"
	"os"
)

const buildDir string = "build"

func ensureBuildDir() error {
	fmt.Printf("Creating output build directory '%v'\n", buildDir)
	st, err := os.Stat(buildDir)
	if os.IsNotExist(err) {
		return os.Mkdir(buildDir, 0755)
	}

	if st.IsDir() {
		return nil
	}

	return fmt.Errorf("cannot create build directory '%v', a file already exists with that name")
}
