package container

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestContainer_Wrangler(t *testing.T) {
	conf := setupWranglerConfig(t)
	var wr *Wrangler
	var err error

	wr, err = NewWrangler(conf)
	require.NoError(t, err)
	require.NotNil(t, wr)

	st, err := os.Stat(wr.lib)
	require.NoError(t, err)
	assert.True(t, st.IsDir(), "expected lib path '%v' to get created by the wrangler", wr.lib)

	st, err = os.Stat(wr.run)
	require.NoError(t, err)
	assert.True(t, st.IsDir(), "expected run path '%v' to get created by the wrangler", wr.run)
}

func TestContainer_Wrangler_GetImage(t *testing.T) {
	tests := []struct {
		dist string
		vers string
	}{
		{"alpine", "3.15"},
		{"alpine", "3.16"},
		{"alpine", ""},
	}

	for i, tt := range tests {
		t.Run(fmt.Sprintf("test_%v", i+1), func(t *testing.T) {
			ctx := context.TODO()
			conf := setupWranglerConfig(t)
			wr, err := NewWrangler(conf)
			require.NoError(t, err)
			require.NotNil(t, wr)

			testSource := tt.dist
			if tt.vers != "" {
				testSource += ":" + tt.vers
			} else {
				testSource += ":" + defaultTag
			}

			var img *Container
			img, err = wr.GetImage(ctx, testSource)
			require.NoError(t, err)
			require.NotNil(t, img)

			assert.Equal(t, testSource, img.Source(), "wrong source")
			assert.Equal(t, tt.dist, img.Distribution(), "wrong distribution")

			if tt.vers != "" {
				assert.Equal(t, tt.vers, img.Version(), "wrong version")
			} else {
				assert.Equal(t, defaultTag, img.Version(), "expected default tag")
			}
		})
	}

}

func setupWranglerConfig(t *testing.T) Config {
	t.Helper()

	tmpDir, err := os.MkdirTemp("./testdata", "wrangler")
	require.NoError(t, err)

	// recommend only skipping cleanup when running single tests!
	skipCleanup := strings.TrimSpace(os.Getenv("TEST_CLEANUP_TMP"))
	if skipCleanup == "" {
		t.Cleanup(func() {
			require.NoError(t, os.RemoveAll(tmpDir))
		})
	}

	err = os.MkdirAll(tmpDir+"/tmp", 0755)
	require.NoError(t, err)

	return Config{
		// would normally be something like /var/lib/worknator
		LibPath: tmpDir + "/lib",
		// would normally be something like /var/run/worknator
		RunPath: tmpDir + "/run",
		// would normally be something like /tmp
		TmpPath: tmpDir + "/tmp",
	}
}
