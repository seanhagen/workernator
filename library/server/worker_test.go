package server

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWorker_NewService(t *testing.T) {
	var svc *Service
	var err error

	svc, err = NewService()
	require.NoError(t, err)
	require.NotNil(t, svc)
}

func TestWorker_Service_Start(t *testing.T) {

}
