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
	fmt.Printf("creating %v test binary...", pkg)

	bin := fmt.Sprintf("%v.test", pkg)

	err := sh.Run("go", "test", "-c", fmt.Sprintf("./%v", pkg))
	if err != nil {
		fmt.Println(" ERROR")
		return fmt.Errorf("unable to create '%v' test binary:\n\t%w", pkg, err)
	}
	fmt.Printf(" DONE: generated test binary '%v'\n", bin)

	fmt.Printf("Running %v tests...", pkg)
	if err := sh.Run(fmt.Sprintf("./%v", bin), "-test.v"); err != nil {
		fmt.Printf(" ERROR\n")
		fmt.Printf("removing test binary '%v'...", bin)
		if e2 := sh.Rm(bin); e2 != nil {
			fmt.Printf("ERROR: unable to remove '%v' test binary, %v", bin, e2)
		}
		fmt.Println(" DONE")

		return err
	}
	fmt.Println(" DONE")

	fmt.Printf("cleaning up after tests...")
	if err := sh.Rm(bin); err != nil {
		fmt.Printf(" ERROR: %v", err)
	}
	fmt.Println(" DONE")

	return nil
}
