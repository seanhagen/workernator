//go:build mage
// +build mage

package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/magefile/mage/mg"
	"github.com/magefile/mage/sh"
)

// Certificates is a shortcut for running ServerCertificates & ClientCertificates
func Certificates() error {
	mg.Deps(CACert, ServerCerts, ClientCerts)
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

	fmt.Printf("[CERT][CA] generating CA certificate '%v'\n", pemPath)

	// generates the key file using:
	//   openssl ecparam -name prime256v1 -genkey -noout -out cakey.key
	err := sh.Run("openssl", "ecparam", "-name", "prime256v1", "-genkey", "-noout", "-out", keyPath)
	if err != nil {
		return fmt.Errorf("unable to generate CA key: %v", err)
	}

	// generate the certificate file using:
	//   openssl req -x509 -new -nodes -config csr.conf -key cakey.key -days 730 -out ca.pem
	err = sh.Run("openssl", "req", "-x509", "-new", "-nodes", "-config", configPath, "-key", keyPath, "-days", "730", "-out", pemPath)
	if err != nil {
		return fmt.Errorf("unable to generate CA certificate: %v", err)
	}

	fmt.Printf("[CERT][CA] done!\n")

	return nil
}

func generateCertificate(certFor string, isClient bool) error {
	mg.Deps(CACert)

	config := "config/csr.conf"
	if isClient {
		config = "config/csrclient.conf"
	}

	fmt.Printf(
		"[CERT][%v] generating certificate using config '%v'\n",
		strings.ToUpper(certFor), config,
	)

	keyPath := buildDir + "/ca.key"
	csrPath := buildDir + "/" + certFor + ".csr"
	err := sh.Run("openssl", "req", "-new", "-key", keyPath,
		"-out", csrPath, "-config", config)
	if err != nil {
		return fmt.Errorf("unable to generate client csr: %v", err)
	}

	caKeyPath := buildDir + "/ca.key"
	caPemPath := buildDir + "/ca.pem"

	pemPath := buildDir + "/" + certFor + ".pem"
	err = sh.Run("openssl", "x509", "-req", "-in", csrPath, "-CA", caPemPath,
		"-CAkey", caKeyPath, "-CAcreateserial", "-out", pemPath, "-days", "90",
		"-extfile", config, "-extensions", "req_ext")
	if err != nil {
		return fmt.Errorf("unable to build '%v', encountered error: %v", pemPath, err)
	}

	err = os.Remove(csrPath)
	if err != nil {
		return fmt.Errorf("unable to remove '%v', encountered error: %v", csrPath, err)
	}

	fmt.Printf("[CERT][%v] done generating certificate\n", strings.ToUpper(certFor))
	return nil
}
