package grpc

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInternal_Config_Valid(t *testing.T) {
	validConfig := Config{
		Port: "8080",

		CertPath:  "./testdata/server.pem",
		KeyPath:   "./testdata/cakey.key",
		ChainPath: "./testdata/ca.pem",

		ACL: UserPermissions{
			"admin": RPCPermissions{
				"start":  Super,
				"stop":   Super,
				"status": Super,
				"output": Super,
			},
		},
	}

	require.NoError(t, validConfig.Valid())

	t.Run("empty config is invalid", func(t *testing.T) {
		conf := Config{}
		assert.Error(t, conf.Valid())
	})

	t.Run("valid port numbers", func(t *testing.T) {
		assert := assert.New(t)

		conf := validConfig
		conf.Port = ""
		assert.Error(conf.Valid())

		conf.Port = "nope"
		assert.Error(conf.Valid())

		conf.Port = "-1"
		assert.Error(conf.Valid())

		conf.Port = "9999999999999999999999"
		assert.Error(conf.Valid())

		conf.Port = "8080"
		assert.NoError(conf.Valid())
	})

	t.Run("valid server cert path", func(t *testing.T) {
		assert := assert.New(t)

		conf := validConfig
		conf.CertPath = ""
		assert.Error(conf.Valid())

		conf.CertPath = "./testdata"
		assert.Error(conf.Valid())

		conf.CertPath = "./testdata/file-does-not-exist"
		assert.Error(conf.Valid())

		conf.CertPath = "./testdata/not-a-cert"
		assert.Error(conf.Valid())

		conf.CertPath = "./testdata/server.pem"
		assert.NoError(conf.Valid())
	})

	t.Run("valid certificate key path", func(t *testing.T) {
		assert := assert.New(t)

		conf := validConfig
		conf.KeyPath = ""
		assert.Error(conf.Valid(), "expected error saying path can't be blank")

		conf.KeyPath = "./testdata"
		assert.Error(conf.Valid(), "expected error saying directory is invalid path")

		conf.KeyPath = "./testdata/file-does-not-exist"
		assert.Error(conf.Valid(), "expected error saying file doesn't exist")

		conf.KeyPath = "./testdata/not-a-cert"
		assert.Error(conf.Valid(), "expected error saying file couldn't be added to pool")

		conf.KeyPath = "./testdata/cakey.key"
		assert.NoError(conf.Valid(), "expected no error")
	})

	t.Run("valid ca chain path", func(t *testing.T) {
		assert := assert.New(t)

		conf := validConfig
		conf.ChainPath = ""
		assert.Error(conf.Valid(), "expected error saying path can't be blank")

		conf.ChainPath = "./testdata"
		assert.Error(conf.Valid(), "expected error saying directory is invalid path")

		conf.ChainPath = "./testdata/file-does-not-exist"
		assert.Error(conf.Valid(), "expected error saying file doesn't exist")

		conf.ChainPath = "./testdata/not-a-cert"
		assert.Error(conf.Valid(), "expected error saying file couldn't be added to pool")

		conf.ChainPath = "./testdata/ca.pem"
		assert.NoError(conf.Valid(), "expected no error from ./testdata/ca.pem")
	})

	t.Run("valid ACL", func(t *testing.T) {
		assert := assert.New(t)

		conf := validConfig
		conf.ACL = UserPermissions{}
		assert.Error(conf.Valid(), "expected empty permissions to return error")

		conf.ACL = UserPermissions{
			"admin": RPCPermissions{},
		}
		assert.Error(conf.Valid(), "expected empty rca permissions for admin to return error")

		conf.ACL = UserPermissions{
			"admin": RPCPermissions{
				"boop": Super,
			},
		}
		assert.Error(conf.Valid(), "expected invalid route name to return error")

		conf.ACL = UserPermissions{
			"admin": RPCPermissions{
				"start": Permission(5),
			},
		}
		assert.Error(conf.Valid(), "expected invalid permission level to return error")

		conf.ACL = UserPermissions{
			"admin": RPCPermissions{
				"start": Super,
			},
		}
		assert.NoError(conf.Valid(), "valid config shouldn't have any errors")
	})
}
