package internal

import (
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"google.golang.org/grpc"
)

// Permission ...
type Permission int32

const (
	// None ...
	None Permission = iota
	// Own ...
	Own
	// Super ...
	Super = 100
)

var validRoutes = []string{"start", "stop", "status", "output"}

// RPCPermissions ...
type RPCPermissions map[string]Permission

// UserPermissions ...
type UserPermissions map[string]RPCPermissions

// Config ...
type Config struct {
	Port string

	CertPath  string
	KeyPath   string
	ChainPath string

	Interceptors struct {
		Unary  []grpc.UnaryServerInterceptor
		Stream []grpc.StreamServerInterceptor
	}

	ACL UserPermissions
}

// Valid ...
func (c Config) Valid() error {
	if err := c.portValid(); err != nil {
		return fmt.Errorf("invalid port: %w", err)
	}

	if err := c.certPathValid(); err != nil {
		return err
	}

	if err := c.keyPathValid(); err != nil {
		return err
	}

	if err := c.chainPathValid(); err != nil {
		return err
	}

	if err := c.aclValid(); err != nil {
		return err
	}

	return nil
}

// portValid  ...
func (c Config) portValid() error {
	c.Port = strings.TrimSpace(c.Port)

	if c.Port == "" {
		return fmt.Errorf("port can't be empty")
	}

	pn, err := strconv.Atoi(c.Port)
	if err != nil {
		return fmt.Errorf("unable to parse port number: %w", err)
	}

	if pn < 1 || pn > 65_535 {
		return fmt.Errorf("port number must be between 1 and 65,535")
	}

	return nil
}

// keyPathValid ...
func (c Config) keyPathValid() error {
	c.KeyPath = strings.TrimSpace(c.KeyPath)
	if err := keyValid(c.KeyPath); err != nil {
		return fmt.Errorf("invalid key: %w", err)
	}

	return nil
}

// certPathValid ...
func (c Config) certPathValid() error {
	c.CertPath = strings.TrimSpace(c.CertPath)
	if err := certValid(c.CertPath); err != nil {
		return fmt.Errorf("invalid certificate: %w", err)
	}

	return nil
}

// chainPathValid ...
func (c Config) chainPathValid() error {
	c.ChainPath = strings.TrimSpace(c.ChainPath)
	if err := certValid(c.ChainPath); err != nil {
		return fmt.Errorf("invalid ca chain certificate: %w", err)
	}
	return nil
}

// aclValid ...
func (c Config) aclValid() error {
	if len(c.ACL) == 0 {
		return fmt.Errorf("acl can't be empty, require at least one configured user")
	}

	for user, rpcPerms := range c.ACL {
		if len(rpcPerms) == 0 {
			return fmt.Errorf("rpc permissions for '%v' are empty", user)
		}

		for route, perm := range rpcPerms {
			if !stringInSlice(route, validRoutes) {
				return fmt.Errorf("'%v' is not a valid route, valid routes: %v", route, validRoutes)
			}
			if perm != None && perm != Own && perm != Super {
				return fmt.Errorf("'%v' is not a valid permissions, valid permissions are None, Own, Super", perm)
			}
		}
	}

	return nil
}

func stringInSlice(s string, sl []string) bool {
	for _, v := range sl {
		if v == s {
			return true
		}
	}
	return false
}

func fileValid(path string) error {
	if path == "" {
		return fmt.Errorf("path can't be blank")
	}

	st, err := os.Stat(path)
	if os.IsNotExist(err) {
		return fmt.Errorf("no file found at '%v'", path)
	}

	if st.IsDir() {
		return fmt.Errorf("'%v' is a directory, not a file", path)
	}

	return nil
}

func keyValid(path string) error {
	if err := fileValid(path); err != nil {
		return fmt.Errorf("invalid key: %w", err)
	}

	read, err := os.OpenFile(path, os.O_RDONLY, 0444)
	if err != nil {
		return fmt.Errorf("unable to open key at '%v', error: %w", path, err)
	}

	bits, err := io.ReadAll(read)
	if err != nil {
		return fmt.Errorf("unable to read key '%v', error: %w", path, err)
	}

	pemBlock, _ := pem.Decode(bits)
	if pemBlock == nil {
		return fmt.Errorf("unable to decode key")
	}

	_, err = x509.ParseECPrivateKey(pemBlock.Bytes)
	if err != nil {
		return fmt.Errorf("unable to parse private key: %w", err)
	}

	return nil
}

func certValid(path string) error {
	if err := fileValid(path); err != nil {
		return fmt.Errorf("invalid certificate: %w", err)
	}

	read, err := os.OpenFile(path, os.O_RDONLY, 0444)
	if err != nil {
		return fmt.Errorf("unable to open certificate at '%v', error: %w", path, err)
	}

	bits, err := io.ReadAll(read)
	if err != nil {
		return fmt.Errorf("unable to read certificate '%v', error: %w", path, err)
	}

	pool := x509.NewCertPool()
	if ok := pool.AppendCertsFromPEM(bits); !ok {
		return fmt.Errorf("unable to append certificate from '%v' to certificate pool", path)
	}

	return nil
}
