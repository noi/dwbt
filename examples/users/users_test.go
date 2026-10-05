package users_test

import (
	"net/http/httptest"
	"testing"

	"github.com/noi/dwbt/dwbttest"
	"github.com/noi/dwbt/examples/users/mockapi"
)

func TestWorkflows(t *testing.T) {
	srv := httptest.NewServer(mockapi.New())
	defer srv.Close()

	dwbttest.New(".dwbt").
		Server("api", srv.URL).
		Run(t)
}
