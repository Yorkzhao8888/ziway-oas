package middleware

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
)

// FileRBACEnforcer implements RBACEnforcer using a model config and CSV policy file.
// Policy CSV format: p, <role>, <resource_pattern>, <http_method>
// Lines starting with '#' are comments. Empty lines are skipped.
// Resource patterns support '*' as wildcard matching any path segment.
// Method '*' matches any HTTP method.
type FileRBACEnforcer struct {
	mu         sync.RWMutex
	rules      []rbacRule
	roles      map[string][]string
	policyPath string
	lastMod    time.Time
}

type rbacRule struct {
	subject  string
	resource string
	action   string
}

// NewFileRBACEnforcer loads model and policy files.
// modelPath is validated for existence (fail-closed); policyPath is parsed for rules.
func NewFileRBACEnforcer(modelPath, policyPath string) (*FileRBACEnforcer, error) {
	if _, err := os.Stat(modelPath); err != nil {
		return nil, fmt.Errorf("rbac model file missing: %s: %w", modelPath, err)
	}
	if _, err := os.Stat(policyPath); err != nil {
		return nil, fmt.Errorf("rbac policy file missing: %s: %w", policyPath, err)
	}

	enforcer := &FileRBACEnforcer{
		roles:      make(map[string][]string),
		policyPath: policyPath,
	}

	if err := enforcer.loadPolicy(policyPath); err != nil {
		return nil, fmt.Errorf("load rbac policy: %w", err)
	}

	return enforcer, nil
}

func (e *FileRBACEnforcer) loadPolicy(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}

	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	var rules []rbacRule
	scanner := bufio.NewScanner(f)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, ",", 4)
		if len(parts) < 4 {
			continue
		}
		typ := strings.TrimSpace(parts[0])
		if typ != "p" {
			continue
		}
		rules = append(rules, rbacRule{
			subject:  strings.TrimSpace(parts[1]),
			resource: strings.TrimSpace(parts[2]),
			action:   strings.TrimSpace(parts[3]),
		})
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("scan policy file: %w", err)
	}

	e.mu.Lock()
	e.rules = rules
	e.lastMod = info.ModTime()
	e.mu.Unlock()
	return nil
}

// Reload re-reads the policy file from disk. Returns true if the file changed.
func (e *FileRBACEnforcer) Reload() (bool, error) {
	info, err := os.Stat(e.policyPath)
	if err != nil {
		return false, err
	}

	e.mu.RLock()
	unchanged := info.ModTime().Equal(e.lastMod)
	e.mu.RUnlock()

	if unchanged {
		return false, nil
	}

	if err := e.loadPolicy(e.policyPath); err != nil {
		return false, err
	}
	return true, nil
}

// WatchFile starts a goroutine that checks for policy file changes every interval.
// Call it once at startup. The goroutine exits when stopCh is closed.
func (e *FileRBACEnforcer) WatchFile(interval time.Duration, stopCh <-chan struct{}, onChange func(ruleCount int)) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-stopCh:
				return
			case <-ticker.C:
				changed, err := e.Reload()
				if err != nil {
					continue
				}
				if changed && onChange != nil {
					e.mu.RLock()
					count := len(e.rules)
					e.mu.RUnlock()
					onChange(count)
				}
			}
		}
	}()
}

// Enforce checks if userID (with given roles) is allowed to access resource with action.
// It checks each role the user has against the policy rules.
func (e *FileRBACEnforcer) Enforce(userID, domain, resource, action string) (bool, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	if len(e.rules) == 0 {
		return false, nil
	}

	for _, rule := range e.rules {
		if matchResource(resource, rule.resource) && matchAction(action, rule.action) {
			if rule.subject == userID {
				return true, nil
			}
		}
	}

	return false, nil
}

// EnforceWithRoles checks access using the user's roles from JWT claims.
func (e *FileRBACEnforcer) EnforceWithRoles(userID string, roles []string, resource, action string) (bool, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	if len(e.rules) == 0 {
		return false, nil
	}

	for _, rule := range e.rules {
		if !matchResource(resource, rule.resource) || !matchAction(action, rule.action) {
			continue
		}
		if rule.subject == userID {
			return true, nil
		}
		for _, role := range roles {
			if rule.subject == role {
				return true, nil
			}
		}
	}

	return false, nil
}

// matchResource checks if the request path matches the policy pattern.
// '*' matches any sequence of characters (glob-style).
func matchResource(request, pattern string) bool {
	if pattern == "*" {
		return true
	}
	if pattern == request {
		return true
	}

	// Convert glob pattern to segment-based matching
	// /api/v1/bos/* matches /api/v1/bos/anything/here
	patParts := strings.Split(pattern, "/")
	reqParts := strings.Split(request, "/")

	return matchSegments(reqParts, patParts)
}

func matchSegments(req, pat []string) bool {
	if len(pat) == 0 {
		return len(req) == 0
	}

	if pat[0] == "*" {
		// '*' at this position matches one or more remaining segments
		for i := 0; i <= len(req); i++ {
			if matchSegments(req[i:], pat[1:]) {
				return true
			}
		}
		return false
	}

	if len(req) == 0 {
		return false
	}

	if pat[0] == req[0] || strings.HasPrefix(pat[0], ":") {
		return matchSegments(req[1:], pat[1:])
	}

	return false
}

// matchAction checks if the request method matches the policy action.
func matchAction(request, pattern string) bool {
	if pattern == "*" {
		return true
	}
	return strings.EqualFold(request, pattern)
}
