package container

import (
	"fmt"
	"strings"
)

func distAndVersionFromSource(source string) (string, string, error) {
	if !strings.Contains(source, ":") {
		return source, defaultTag, nil
	}

	parts := strings.Split(source, ":")
	if len(parts) < 2 {
		return "", "", fmt.Errorf("source missing tag, got '%v'; expected string after ':'", source)
	}

	return parts[0], parts[1], nil
}
