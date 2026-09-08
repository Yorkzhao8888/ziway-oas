// helpers.go — 自 cmd/oas/main.go 下沉（OAS-CONSOLE-09 A1）。
package handlers

import "fmt"

func parseInt(s string) (int, error) {
	var n int
	_, err := fmt.Sscanf(s, "%d", &n)
	return n, err
}

func parseUint(s string) (uint64, error) {
	var n uint64
	_, err := fmt.Sscanf(s, "%d", &n)
	return n, err
}

func ptrUint64(n uint64) *uint64 {
	return &n
}

func ptrString(s string) *string {
	return &s
}
