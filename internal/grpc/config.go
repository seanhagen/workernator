package grpc

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
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

// valid  ...
func (rpcPerms RPCPermissions) valid(user string) error {
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
	return nil
}

// UserPermissions maps user names to a set of RPC permissions
type UserPermissions map[string]RPCPermissions

// valid  ...
func (userPerms UserPermissions) valid() error {
	if len(userPerms) == 0 {
		return fmt.Errorf("acl can't be empty, require at least one configured user")
	}

	for user, rpcPerms := range userPerms {
		if err := rpcPerms.valid(user); err != nil {
			return err
		}
	}
	return nil
}

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
		Unary  []UnaryServerInterceptor
		Stream []StreamServerInterceptor
	}

	// ACL is the set of user permissions the server will use
	ACL UserPermissions
}

// Valid tests the config to ensure it's valid. If it's not valid, an
// error is returned.
func (c Config) Valid() error {
	if err := c.portValid(); err != nil {
		return fmt.Errorf("invalid port: %w", err)
	}

	if err := c.ACL.valid(); err != nil {
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

// validateCertificate ...
func (c Config) validateCertificate() (tls.Certificate, error) {
	return tls.LoadX509KeyPair(c.CertPath, c.KeyPath)
}

// validateChain ...
func (c Config) validateChain() (*x509.CertPool, error) {
	chainReader, err := os.OpenFile(c.ChainPath, os.O_RDONLY, 0444)
	if err != nil {
		return nil, fmt.Errorf("unable to open chain file: %w", err)
	}

	bits, err := io.ReadAll(chainReader)
	if err != nil {
		return nil, fmt.Errorf("unable to read from chain file: %w", err)
	}
	certPool := x509.NewCertPool()
	if ok := certPool.AppendCertsFromPEM(bits); !ok {
		return nil, fmt.Errorf("unable to append cert from '%v' to cert pool", c.ChainPath)
	}

	return certPool, nil
}

func stringInSlice(s string, sl []string) bool {
	for _, v := range sl {
		if v == s {
			return true
		}
	}
	return false
}
