//go:build mage
// +build mage

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/magefile/mage/mg" // mg contains helpful utility functions, like Deps
	"github.com/magefile/mage/sh"
	"github.com/magefile/mage/target"
)

const protobufLatestRelease = "https://api.github.com/repos/protocolbuffers/protobuf/releases/latest"

const (
	grpcProtobufIn    = "proto/workernator.proto"
	grpcDescriptorOut = "internal/pb/grpc_descriptor.pb"
	grpcServiceOut    = "internal/pb/workernator_grpc.pb.go"
	grpcDataOut       = "internal/pb/workernator.pb.go"
)

// GRPC will download the latest GRPC protobuf release so it can get
// the Google GRPC definition files, and put them in `proto/vendor`,
// then it will generate the GRPC descriptor & other GRPC files.
func GRPC() error {
	inputs := []string{
		grpcProtobufIn,
	}

	outputs := []string{
		grpcServiceOut,
		grpcDataOut,
	}

	mod, err := checkNewer(outputs, inputs)
	if err != nil {
		return err
	}
	if !mod {
		fmt.Printf("none of %v are newer than %v, not auto-generating GRPC code\n", inputs, outputs)
		return nil
	}
	mg.Deps(grpcDescriptor, grpcVendor)

	fmt.Printf("%v is newer than %v (or some of those files haven't been generated), auto-generating GRPC code\n", inputs, outputs)

	return sh.Run("protoc", "-I=proto", "-I=proto/vendor",
		"--go_out=internal/pb",
		"--go_opt=paths=source_relative",
		"--go-grpc_out=internal/pb",
		"--go-grpc_opt=paths=source_relative",
		"workernator.proto")
}

func grpcVendor() error {
	outDirExist, e1 := dirExists("proto/vendor")
	anOutputFile, e2 := fileExists("proto/vendor/timestamp.proto")

	if outDirExist && anOutputFile && e1 == nil && e2 == nil {
		fmt.Println("grpc vendor folder and protobuf files already exist, not re-downloading")
		return nil
	}

	fmt.Println("downloading the latest google protobuf vendor files")

	resp, err := http.Get(protobufLatestRelease)
	if err != nil {
		return fmt.Errorf("unable to get latest release info from github: %w", err)
	}

	var data map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&data)
	if err != nil {
		return fmt.Errorf("unable to parse release JSON body: %w", err)
	}
	_ = resp.Body.Close()

	url, ok := data["tarball_url"].(string)
	if !ok {
		return fmt.Errorf("unable to cast url to string")
	}

	fmt.Printf("downloading tarball from '%v'\n", url)

	output, err := os.OpenFile("protobuf.tar.gz", os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("unable to open output file: %w", err)
	}

	resp, err = http.Get(url)
	if err != nil {
		return fmt.Errorf("unable to get protobuf tarball: %w", err)
	}

	fmt.Println("copying to destination...")

	wr, err := io.Copy(output, resp.Body)
	if err != nil {
		return fmt.Errorf("unable to write tarball to output: %w", err)
	}

	if err = output.Close(); err != nil {
		return fmt.Errorf("unable to close output file: %w", err)
	}

	file, err := os.Open("protobuf.tar.gz")
	if err != nil {
		return fmt.Errorf("unable to open tarball: %w", err)
	}

	fmt.Printf("wrote %v bytes\n", wr)

	err = extractFilesFromTar(
		file,
		[]string{
			"any.proto",
			"api.proto",
			"duration.proto",
			"empty.proto",
			"timestamp.proto",
		},
		"proto/vendor",
	)
	if err != nil {
		return fmt.Errorf("unable to extract files from tarball: %w", err)
	}

	return os.Remove("protobuf.tar.gz")
}

func grpcDescriptor() error {
	mg.Deps(grpcVendor)

	mod, err := target.Path(grpcDescriptorOut, grpcProtobufIn)
	if err != nil {
		return err
	}
	if !mod {
		fmt.Printf("'%v' is newer than '%v', not rebuilding\n", grpcDescriptorOut, grpcProtobufIn)
		return nil
	}

	fmt.Printf("building '%v' from '%v'\n", grpcDescriptorOut, grpcProtobufIn)
	return sh.Run(
		"protoc", "-I=proto", "-I=proto/vendor", "--include_imports",
		"--descriptor_set_out=internal/pb/grpc_desciptor.pb",
		"workernator.proto")
}
