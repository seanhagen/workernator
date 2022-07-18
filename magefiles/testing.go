package main

import (
	"fmt"

	"github.com/magefile/mage/sh"
)

// Test runs the
func Test() error {
	fmt.Printf("Running tests...")
	err := sh.Run("go", "test", "./...")
	if err != nil {
		fmt.Printf(" error while running tests: %v\n", err)
		return err
	}
	fmt.Printf(" done!\n")
	return nil
}
