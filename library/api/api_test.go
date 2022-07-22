package api

import (
	"context"
	"io"
	"io/ioutil"
	"os"
	"strings"
	"testing"

	"github.com/rs/xid"
	"github.com/seanhagen/workernator/library"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLibrary_NewManager(t *testing.T) {
	path := t.TempDir()
	conf := Config{
		OutputPath: path,
	}

	var mng *Manager
	var err error
	mng, err = NewManager(conf)
	require.NoError(t, err)
	require.NotNil(t, mng)
}

func TestLibrary_Manager_StartJob(t *testing.T) {
	path := t.TempDir()
	conf := Config{
		OutputPath: path,
	}

	mng, err := NewManager(conf)
	require.NoError(t, err)
	require.NotNil(t, mng)

	ctx := context.TODO()
	command := "/usr/bin/echo"
	args := []string{"hey"}

	t.Run("command that doesn't exist should fail", func(t *testing.T) {
		job, err := mng.StartJob(ctx, "/nope/does/not/exist", "hahaha")
		require.Error(t, err)
		require.Nil(t, job)
	})

	t.Run("commands that require arguments should return an error if not given any", func(t *testing.T) {
		jerb, err := mng.StartJob(ctx, "awk")
		require.NoError(t, err)
		require.NotNil(t, jerb)

		err = jerb.Wait()
		require.Error(t, err)
		assert.Equal(t, err, jerb.Error)
		require.True(t, strings.Contains(jerb.Error.Error(), "exit status 1"))
	})

	t.Run("commands with invalid arguments should return an error", func(t *testing.T) {
		jerb, err := mng.StartJob(ctx, "cat", "./testdata/nope-does-not-exist")
		require.NoError(t, err)
		require.NotNil(t, jerb)

		err = jerb.Wait()
		require.Error(t, err)
		require.True(t, strings.Contains(err.Error(), "exit status 1"))
	})

	t.Run("properly set up command should run", func(t *testing.T) {
		job, err := mng.StartJob(ctx, command, args...)
		require.NoError(t, err)
		require.NotNil(t, job)

		_, err = xid.FromString(job.ID)
		assert.NoError(t, err)

		assert.Equal(t, command, job.Command)
		assert.Equal(t, args, job.Arguments)

		assert.NoError(t, job.Wait())
		assert.Equal(t, library.Finished, job.Status)
	})
}

func TestLibrary_Manager_StopJob(t *testing.T) {
	path := t.TempDir()
	conf := Config{
		OutputPath: path,
	}

	mng, err := NewManager(conf)
	require.NoError(t, err)
	require.NotNil(t, mng)

	ctx := context.TODO()
	command := "/usr/bin/sleep"
	args := []string{"40"}

	t.Run("calling StopJob with invalid ID should return an error", func(t *testing.T) {
		job, err := mng.StopJob(ctx, "nope")
		require.Error(t, err)
		require.Nil(t, job)
	})

	t.Run("trying to stop a non-existent job should return an error", func(t *testing.T) {
		id := xid.New()
		job, err := mng.StopJob(ctx, id.String())
		require.Error(t, err)
		require.Nil(t, job)
	})

	t.Run(
		"should be able top stop a job that has been started with (*Manager).StopJob(context.Context, string)",
		func(t *testing.T) {
			job, err := mng.StartJob(ctx, command, args...)
			require.NoError(t, err)
			require.NotNil(t, job)

			jobInfo, err := mng.StopJob(ctx, job.ID)
			require.NoError(t, err)
			require.NotNil(t, jobInfo)
			assert.Equal(t, library.Stopped, jobInfo.Status)
		},
	)

	t.Run("should be able to stop a job by calling (Job).Stop()", func(t *testing.T) {
		job, err := mng.StartJob(ctx, command, args...)
		require.NoError(t, err)
		require.NotNil(t, job)

		err = job.Stop()
		require.NoError(t, err)
		assert.Equal(t, library.Stopped, job.Status)
	})
}

func TestLibrary_Manager_JobStatus(t *testing.T) {
	path := t.TempDir()
	conf := Config{
		OutputPath: path,
	}

	mng, err := NewManager(conf)
	require.NoError(t, err)
	require.NotNil(t, mng)

	ctx := context.TODO()
	command := "/usr/bin/sleep"
	args := []string{"40"}

	t.Run("invalid id, should get error", func(t *testing.T) {
		_, err := mng.JobStatus(ctx, "someid")
		require.Error(t, err)
	})

	t.Run("no jobs, should get error", func(t *testing.T) {
		id := xid.New()
		_, err := mng.JobStatus(ctx, id.String())
		require.Error(t, err)
	})

	t.Run("after running job, should get status of that job", func(t *testing.T) {
		skipSlow := getLowerCaseEnvVar("SKIP_SLOW_TESTS")
		if skipSlow != "" {
			t.Skip("TEST_ALLOW_SLOW environment variable set")
		}

		job, err := mng.StartJob(ctx, command, args...)
		require.NoError(t, err)
		require.NotNil(t, job)

		status, err := mng.JobStatus(ctx, job.ID)
		require.NoError(t, err)
		require.NotNil(t, status)

		assert.Equal(t, job.ID, status.ID)
		assert.Equal(t, job.Command, status.Command)
		assert.Equal(t, job.Arguments, status.Arguments)
	})
}

func TestLibrary_Manager_Output(t *testing.T) {
	skipSlow := getLowerCaseEnvVar("SKIP_SLOW_TESTS")
	if skipSlow != "" {
		t.Skip("TEST_ALLOW_SLOW environment variable set")
	}

	path := t.TempDir()
	conf := Config{
		OutputPath: path,
	}

	mng, err := NewManager(conf)
	require.NoError(t, err)
	require.NotNil(t, mng)

	ctx := context.TODO()
	command := "cat"
	args := []string{"testdata/catme"}

	t.Run("invalid id, should get error", func(t *testing.T) {
		_, err := mng.GetJobOutput(ctx, "someid")
		require.Error(t, err)
	})

	t.Run("no jobs, should get error", func(t *testing.T) {
		id := xid.New()
		_, err := mng.GetJobOutput(ctx, id.String())
		require.Error(t, err)
	})

	t.Run("output data is what we expect", func(t *testing.T) {
		job, err := mng.StartJob(ctx, command, args...)
		require.NoError(t, err)
		require.NotNil(t, job)

		// wait for the job to complete
		require.NoError(t, job.Wait())

		reader, err := mng.GetJobOutput(ctx, job.ID)
		require.NoError(t, err)
		var r *io.Reader
		require.Implements(t, r, reader)

		gotData, err := ioutil.ReadAll(reader)
		require.NoError(t, err)

		bits, err := ioutil.ReadFile("./testdata/catme")
		require.NoError(t, err)
		expect := string(bits)
		got := string(gotData)
		assert.Equal(t, expect, got)
	})

	t.Run("job tails the output properly and we get everything", func(t *testing.T) {
		expect := `hello teleport
here i am
another line
boop
all done!
`

		command = "testdata/slow_output.sh"
		args = []string{"teleport"}

		job, err := mng.StartJob(ctx, command, args...)
		require.NoError(t, err)
		require.NotNil(t, job)

		reader, err := mng.GetJobOutput(ctx, job.ID)
		require.NoError(t, err)
		var r *io.Reader
		require.Implements(t, r, reader)

		gotData, err := ioutil.ReadAll(reader)
		require.NoError(t, err)

		got := string(gotData)
		assert.Equal(t, expect, got)

	})
}

func getLowerCaseEnvVar(name string) string {
	return strings.ToLower(
		strings.TrimSpace(
			os.Getenv(name),
		),
	)
}
