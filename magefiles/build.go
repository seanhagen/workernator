//go:build mage
// +build mage

package main

import (
	"bytes"
	"fmt"
	"os"

	"github.com/magefile/mage/mg"
	"github.com/magefile/mage/sh"
)

// All does what it says on the tin: builds the client & server
// binaries, and generates the TLS certificates
func All() error {
	mg.SerialDeps(Lint, Test, Certificates, Build)
	return nil
}

// Build generates both the server and CLI client binaries
func Build() error {
	mg.SerialDeps(BuildServer, BuildClient)
	return nil
}

// BuildClient builds the CLI client binary
func BuildClient() error {
	mg.SerialDeps(ensureBuildDir, Lint)

	fmt.Print("[BUILD][CLIENT] building client...")
	binaryOut := buildDir + "/client"
	err := sh.Run("go", "build", "-o", binaryOut, "./cmd/client")
	if err != nil {
		fmt.Println(" ERROR")
		return err
	}
	fmt.Println(" SUCCESS")

	fmt.Print("[BUILD][CLIENT] using upx to shrink the binary size")
	err = sh.Run("upx", binaryOut)
	if err != nil {
		fmt.Println(" ERROR")
		return err
	}
	fmt.Println(" SUCCESS")
	return nil
}

// BuildServer builds the server binary
func BuildServer() error {
	mg.SerialDeps(ensureBuildDir, Lint, buildServerBinary)

	fmt.Print("[BUILD][SERVER] using upx to shrink the binary size... ")
	binaryOut := buildDir + "/server"
	err := sh.Run("upx", binaryOut)
	if err != nil {
		fmt.Println(" ERROR")
		return err
	}
	fmt.Println(" SUCCESS")
	return nil
}

// QuickServer builds the server binary without linting
func QuickServer() error {
	mg.SerialDeps(ensureBuildDir)
	return buildServerBinary()
}

func buildServerBinary() error {
	mg.SerialDeps(ensureBuildDir)

	fmt.Print("[BUILD][SERVER] building server...")
	binaryOut := buildDir + "/server"
	//sh.Exec(env map[string]string, stdout io.Writer, stderr io.Writer, cmd string, args ...string)

	env := map[string]string{
		"CGO_ENABLED": "0",
		"GOOS":        "linux",
	}

	_, err := sh.Exec(env, os.Stdout, os.Stderr,
		"go", "build", "-o", binaryOut, "./cmd/server",
	)
	//err := sh.Run("go", "build", "-o", binaryOut, "./cmd/server")
	if err != nil {
		fmt.Println(" ERROR")
		return err
	}
	fmt.Println(" SUCCESS")
	return nil
}

// Lint runs golangci-lint on the code
func Lint() error {
	stdOut := bytes.NewBuffer(nil)
	stdErr := bytes.NewBuffer(nil)

	fmt.Fprintf(os.Stdout, "[BUILD][LINT] linting the code...")
	_, err := sh.Exec(nil, stdOut, stdErr, "golangci-lint", "run", "-v", "-c", ".golangci.yml", "./...")
	if err != nil {
		fmt.Fprintf(os.Stdout, " ERROR!\n")
		fmt.Fprintf(os.Stdout, stdOut.String())
		fmt.Fprintf(os.Stderr, stdErr.String())
		return err
	}
	fmt.Fprintf(os.Stdout, " SUCCESS!\n")
	return nil
}
