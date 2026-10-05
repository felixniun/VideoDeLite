package task

import (
	"errors"
	"os"
	"strings"
)

func baseNameOf(p string) string {
	p = strings.ReplaceAll(p, "\\", "/")
	if i := strings.LastIndex(p, "/"); i >= 0 {
		p = p[i+1:]
	}
	return p
}

func statFile(p string) (int64, error) {
	st, err := os.Stat(p)
	if err != nil {
		return 0, err
	}
	return st.Size(), nil
}

var errDiskStatUnsupported = errors.New("disk stat unsupported on this platform")
