package workernator

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testRootFS = "./testdata/busyboxfs"

// TestMain is being used so we can catch that we've been
// forked/cloned/reexeced. We need to catch that, otherwise the tests
// don't work so great.
func TestMain(m *testing.M) {
	if len(os.Args) > 0 && os.Args[0] == runInNamespace {
		if err := runningInContainer(); err != nil {
			fmt.Fprintf(os.Stderr, "error while running: %v\n", err)
			os.Exit(1)
		}
		os.Exit(0)
	}

	os.Exit(m.Run())
}

func TestRunner_Container_Basics(t *testing.T) {
	tests := []struct {
		rootFS    string
		stdin     *bytes.Buffer
		command   string
		args      []string
		expectOut string
		expectErr string
	}{
		{ // basic test, full path for the binary being called
			rootFS:    testRootFS,
			command:   "/bin/echo",
			args:      []string{"-n", "hey"},
			expectOut: "hey",
		},
		{ // still pretty basic, but relying on the PATH env var so 'echo' can run
			rootFS:    testRootFS,
			command:   "echo",
			args:      []string{"-n", "hey"},
			expectOut: "hey",
		},
		{ // now let's test to see if stdin is working
			rootFS:    testRootFS,
			stdin:     bytes.NewBuffer([]byte("testing")),
			command:   "sed",
			args:      []string{"-e", "s/testing/hey/"},
			expectOut: "hey",
		},
		{ // super minimal root fs, running a binary file
			rootFS:    "./testdata/simplefs",
			command:   "./simple",
			expectOut: "hello world!",
		},
	}

	for i, tt := range tests {
		t.Run(fmt.Sprintf("test_%v", i), func(t *testing.T) {
			assert := assert.New(t)
			require := require.New(t)

			var stdin *bytes.Buffer = tt.stdin
			if stdin == nil {
				stdin = bytes.NewBuffer(nil)
			}

			// if run via `go test`, the working directory is correct, but if we run
			// the binary generated via `go test -c`, the working directory is
			// wherever that binary was run.
			rootfs := getRootFS(t, tt.rootFS)

			stdout := bytes.NewBuffer(nil)
			stderr := bytes.NewBuffer(nil)

			c, err := NewContainer(stdin, stdout, stderr, tt.command, tt.args, rootfs)
			require.NoError(err)
			require.NotNil(c)

			ctx := context.TODO()
			err = c.Run(ctx)

			outStr := stdout.String()
			errStr := stderr.String()

			require.NoError(err, "output: '%v'\nerror: '%v'\n", outStr, errStr)

			assert.Equal(tt.expectOut, outStr)
			assert.Equal(tt.expectErr, errStr)
		})
	}
}

func getRootFS(t *testing.T, root string) string {
	t.Helper()
	wd, err := os.Getwd()
	require.NoError(t, err)

	rootfs, err := filepath.Abs(root)
	require.NoError(t, err)
	if filepath.Base(wd) == "library" {
		rootfs = strings.Replace(rootfs, "/library", "", 1)
	}
	return rootfs
}
