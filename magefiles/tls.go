//go:build mage
// +build mage

package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/magefile/mage/mg"
	"github.com/magefile/mage/sh"
)

// Certificates is a shortcut for running ServerCertificates & ClientCertificates
func Certificates() error {
	mg.SerialDeps(CACert, ServerCerts, ClientCerts)
	return nil
}

// ServerCerts generates the the TLS certificate for the server
// and puts it in build/server.pem
func ServerCerts() error {
	return generateCertificate("server", false)
}

// ClientCerts generates the TLS certificates for the following users:
// admin, alice, bob, charlie. These certificates are placed in the
// 'build' folder.
func ClientCerts() error {
	clients := []string{"admin", "alice", "bob", "charlie"}
	for _, c := range clients {
		if err := generateCertificate(c, true); err != nil {
			return err
		}
	}
	return nil
}

// CACert generates the certificate authority (CA) certificate,
// required to generate any server or client certificates
func CACert() error {
	mg.Deps(ensureBuildDir)

	keyPath := buildDir + "/ca.key"
	pemPath := buildDir + "/ca.pem"
	configPath := "config/csr.conf"

	fmt.Fprintf(os.Stdout, "[CERT][CA] generating CA certificate '%v'...", pemPath)

	stdOut := bytes.NewBuffer(nil)
	stdErr := bytes.NewBuffer(nil)

	// generates the key file using:
	//   openssl ecparam -name prime256v1 -genkey -noout -out cakey.key
	_, err := sh.Exec(nil, stdOut, stdErr, "openssl", "ecparam", "-name", "prime256v1", "-genkey", "-noout", "-out", keyPath)
	if err != nil {
		fmt.Fprintf(os.Stdout, " ERROR: %v\n", err)
		fmt.Fprintf(os.Stdout, stdOut.String())
		fmt.Fprintf(os.Stderr, stdErr.String())
		return fmt.Errorf("unable to generate CA key: %v", err)
	}

	stdOut.Truncate(0)
	stdErr.Truncate(0)

	// generate the certificate file using:
	//   openssl req -x509 -new -nodes -config csr.conf -key cakey.key -days 730 -out ca.pem
	_, err = sh.Exec(nil, stdOut, stdErr, "openssl", "req", "-x509", "-new", "-nodes",
		"-config", configPath, "-key", keyPath, "-days", "730", "-out", pemPath)
	if err != nil {
		fmt.Fprintf(os.Stdout, " ERROR: %v\n", err)
		fmt.Fprintf(os.Stdout, stdOut.String())
		fmt.Fprintf(os.Stderr, stdErr.String())
		return fmt.Errorf("unable to generate CA certificate: %v", err)
	}
	fmt.Fprintf(os.Stdout, " SUCCESS\n")

	return nil
}

func generateCertificate(certFor string, isClient bool) error {
	mg.Deps(CACert)

	config := "config/csr.conf"
	if isClient {
		config = "config/csrclient.conf"
	}

	fmt.Fprintf(
		os.Stdout,
		"[CERT][%s] generating certificate using config '%s'",
		strings.ToUpper(certFor),
		config,
	)

	keyPath := buildDir + "/ca.key"
	csrPath := buildDir + "/" + certFor + ".csr"
	stdOut := bytes.NewBuffer(nil)
	stdErr := bytes.NewBuffer(nil)
	_, err := sh.Exec(nil, stdOut, stdErr, "openssl", "req", "-new", "-key", keyPath,
		"-out", csrPath, "-config", config)
	if err != nil {
		fmt.Fprintf(os.Stdout, " ERROR: %v\n", err)
		fmt.Fprintf(os.Stdout, stdOut.String())
		fmt.Fprintf(os.Stderr, stdErr.String())
		return fmt.Errorf("unable to generate client csr: %v", err)
	}

	caKeyPath := buildDir + "/ca.key"
	caPemPath := buildDir + "/ca.pem"

	pemPath := buildDir + "/" + certFor + ".pem"
	stdOut.Truncate(0)
	stdErr.Truncate(0)
	_, err = sh.Exec(nil, io.Discard, stdErr, "openssl", "x509", "-req", "-in", csrPath, "-CA", caPemPath,
		"-CAkey", caKeyPath, "-CAcreateserial", "-out", pemPath, "-days", "90",
		"-extfile", config, "-extensions", "req_ext")
	if err != nil {
		fmt.Fprintf(os.Stdout, " ERROR: %v\n", err)
		fmt.Fprintf(os.Stdout, stdOut.String())
		fmt.Fprintf(os.Stderr, stdErr.String())
		return fmt.Errorf("unable to build '%v', encountered error: %v", pemPath, err)
	}

	err = os.Remove(csrPath)
	if err != nil {
		return fmt.Errorf("unable to remove '%v', encountered error: %v", csrPath, err)
	}

	fmt.Printf(" SUCCESS\n")
	return nil
}
