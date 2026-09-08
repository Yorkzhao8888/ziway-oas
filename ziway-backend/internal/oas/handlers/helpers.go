// helpers.go — 自 cmd/oas/main.go 下沉（OAS-CONSOLE-09 A1）。
package handlers

import "strconv"

func parseInt(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}

func parseUint(s string) uint {
	n, _ := strconv.ParseUint(s, 10, 64)
	return uint(n)
}

func ptrUint64(n uint64) *uint64 {
	return &n
}

func ptrString(s string) *string {
	return &s
}
