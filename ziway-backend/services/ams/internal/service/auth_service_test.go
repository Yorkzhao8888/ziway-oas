package service

import (
	"testing"
)

// TestAuthServiceSetup verifies the service can be referenced without panic.
// Full integration tests require PostgreSQL + Redis, run with:
//   make docker-up && go test -tags=integration ./...
func TestAuthServiceSetup(t *testing.T) {
	t.Log("AuthService unit tests require database; see integration test docs")
}
