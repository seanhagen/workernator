//go:build mage
// +build mage

package main

import (
	"fmt"

	"github.com/magefile/mage/mg"
	"github.com/magefile/mage/sh"
)

// All does what it says on the tin: builds the client & server
// binaries, and generates the TLS certificates
func All() error {
	mg.Deps(Test, Build, Certificates)
	return nil
}

// Build generates both the server and CLI client binaries
func Build() error {
	mg.Deps(BuildServer, BuildClient)
	return nil
}

// BuildClient builds the CLI client binary
func BuildClient() error {
	mg.Deps(ensureBuildDir)

	fmt.Println("[BUILD][CLIENT] building client...")

	binaryOut := buildDir + "/client"
	err := sh.Run("go", "build", "-o", binaryOut, "./cmd/client")
	if err != nil {
		fmt.Printf(" ERROR\n")
		return err
	}
	return nil
}

// BuildServer builds the server binary
func BuildServer() error {
	mg.Deps(ensureBuildDir)

	fmt.Println("building server...")

	binaryOut := buildDir + "/server"
	return sh.Run("go", "build", "-o", binaryOut, "./cmd/server")
}

// // A build step that requires additional params, or platform specific steps for example
// func Build() error {
// 	mg.Deps(InstallDeps)
// 	fmt.Println("Building...")
// 	cmd := exec.Command("go", "build", "-o", "MyApp", ".")
// 	return cmd.Run()
// }

// // A custom install step if you need your bin someplace other than go/bin
// func Install() error {
// 	mg.Deps(Build)
// 	fmt.Println("Installing...")
// 	return os.Rename("./MyApp", "/usr/bin/MyApp")
// }

// // Manage your deps, or running package managers.
// func InstallDeps() error {
// 	fmt.Println("Installing Deps...")
// 	cmd := exec.Command("go", "get", "github.com/stretchr/piglatin")
// 	return cmd.Run()
// }

// // Clean up after yourself
// func Clean() {
// 	fmt.Println("Cleaning...")
// 	os.RemoveAll("MyApp")
// }
