package cli

import (
	"os"
	"path/filepath"
)

// defaultCwdBasename returns the base name of the current working directory,
// used to derive a default project slug when --slug is not provided.
func defaultCwdBasename() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return filepath.Base(cwd), nil
}
