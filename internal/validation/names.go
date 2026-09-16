package validation

import (
	"fmt"
	"strings"
	"unicode"
)

func Name(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || name == "." || name == ".." || len([]rune(name)) > 255 {
		return "", fmt.Errorf("invalid name")
	}
	for _, r := range name {
		if r == '/' || r == '\\' || r == 0 || unicode.IsControl(r) {
			return "", fmt.Errorf("invalid name")
		}
	}
	return name, nil
}
