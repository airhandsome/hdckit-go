package hdc

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func getLastPid() int {
	p := filepath.Join(os.TempDir(), ".HDCServer.pid")
	b, err := os.ReadFile(p)
	if err != nil {
		return 0
	}
	s := strings.TrimSpace(string(b))
	v, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}
	return v
}
