package server

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWorker_Start(t *testing.T) {
	mgr := testManager{}
	svc, err := NewService(mgr)
	require.NoError(t, err)
	require.NotNil(t, svc)
}
