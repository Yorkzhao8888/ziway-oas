package envpolicy

import (
	"os"
	"strings"
)

// Environment represents the OAS ecosystem stage
type Environment string

const (
	DEV  Environment = "DEV"
	BETA Environment = "BETA"
	RC   Environment = "RC"
	PROD Environment = "PROD"
)

// GetOASEnv reads OAS_ENV from environment, defaults to DEV
func GetOASEnv() Environment {
	env := os.Getenv("OAS_ENV")
	if env == "" {
		return DEV
	}
	env = strings.ToUpper(strings.TrimSpace(env))
	switch Environment(env) {
	case DEV, BETA, RC, PROD:
		return Environment(env)
	default:
		return DEV
	}
}

// IsQuickLoginEnabled returns true if quick-login should be available
// Only DEV and BETA environments allow quick-login
func IsQuickLoginEnabled(env Environment) bool {
	return env == DEV || env == BETA
}

// IsTestAccountsEnabled returns true if test-accounts endpoint should be available
// Only DEV and BETA environments allow test-accounts
func IsTestAccountsEnabled(env Environment) bool {
	return env == DEV || env == BETA
}

// IsDevTokenEnabled returns true if dev-token should be available
// Only DEV environment allows dev-token (PROD always disabled, RC/BETA also disabled for safety)
func IsDevTokenEnabled(env Environment) bool {
	return env == DEV
}

// String returns the string representation
func (e Environment) String() string {
	return string(e)
}
