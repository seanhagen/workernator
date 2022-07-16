package main

import (
	"fmt"

	"github.com/magefile/mage/sh"
)

func Test() error {
	fmt.Printf("Running tests...\n")
	packages := []string{"library"}

	for _, p := range packages {
		if err := runSubtests(p); err != nil {
			return err
		}
	}

	return nil
}

func runSubtests(pkg string) error {
	fmt.Printf("Running tests for '%v'...", pkg)
	err := sh.Run("go", "test", fmt.Sprintf("./%v", pkg))
	if err != nil {
		fmt.Printf(" ERROR\n")
		return err
	}
	fmt.Printf(" DONE\n")
	return nil
}
