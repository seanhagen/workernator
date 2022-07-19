package main

import (
	"bytes"
	"fmt"
	"os"

	"github.com/magefile/mage/sh"
)

// Test runs the go tests and reports any errors
func Test() error {
	stdOut := bytes.NewBuffer(nil)
	stdErr := bytes.NewBuffer(nil)

	fmt.Fprintf(os.Stdout, "[TESTS][RUN] Running tests...")
	_, err := sh.Exec(nil, stdOut, stdErr, "go", "test", "./...")
	if err != nil {
		fmt.Fprintf(os.Stdout, " ERROR: %v\n", err)
		fmt.Fprintf(os.Stdout, stdOut.String())
		fmt.Fprintf(os.Stderr, stdErr.String())
		return err
	}
	fmt.Fprintf(os.Stdout, " SUCCESS\n")
	return nil
}
