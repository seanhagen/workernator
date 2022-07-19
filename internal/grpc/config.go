package grpc

import (
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	grpc_zap "github.com/grpc-ecosystem/go-grpc-middleware/logging/zap"
	"google.golang.org/grpc"
)

// Permission defines a type used to handle permission levels
type Permission int32

const (
	// None means the user cannot use the RPC
	None Permission = 0
	// Own means the user can use the RPC, but only to interact with their own jobs
	Own = 1
	// Super means the user can use the RPC, and can interact with any jobs
	Super = 100
)

var validRoutes = []string{"start", "stop", "status", "output"}

// RPCPermissions maps RPC names to a permission level
type RPCPermissions map[string]Permission

// UserPermissions maps user names to a set of RPC permissions
type UserPermissions map[string]RPCPermissions

// Config contains the information required to set up a GRPC server
type Config struct {
	// DevMode debug logging will be enabled if true
	DevMode bool
	// Port controls the port the GRPC server will run on
	Port string

	// CertPath should be the path to a valid TLS v1.3 certificate file
	CertPath string
	// ChainPath should be the path to the valid TLS v1.3 CA certificate
	// used to generate the certificate found in CertPath
	ChainPath string
	// KeyPath should be the path to the key used in generating both
	// certificates
	KeyPath string

	// Interceptors is optional, but any interceptors in here will be
	// added to the list of interceptors during server startup
	Interceptors struct {
		Unary  []grpc.UnaryServerInterceptor
		Stream []grpc.StreamServerInterceptor
	}

	// ACL is the set of user permissions the server will use
	ACL UserPermissions

	LogOpts []grpc_zap.Option
}

// Valid tests the config to ensure it's valid. If it's not valid, an
// error is returned.
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

func (c Config) keyPathValid() error {
	c.KeyPath = strings.TrimSpace(c.KeyPath)
	if err := keyValid(c.KeyPath); err != nil {
		return fmt.Errorf("invalid key: %w", err)
	}

	return nil
}

func (c Config) certPathValid() error {
	c.CertPath = strings.TrimSpace(c.CertPath)
	if err := certValid(c.CertPath); err != nil {
		return fmt.Errorf("invalid certificate: %w", err)
	}

	return nil
}

func (c Config) chainPathValid() error {
	c.ChainPath = strings.TrimSpace(c.ChainPath)
	if err := certValid(c.ChainPath); err != nil {
		return fmt.Errorf("invalid ca chain certificate: %w", err)
	}
	return nil
}

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
