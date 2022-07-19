package server

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWorker_Start(t *testing.T) {
	svc, err := NewService()
	require.NoError(t, err)
	require.NotNil(t, svc)
}
