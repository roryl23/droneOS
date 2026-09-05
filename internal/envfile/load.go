// Package envfile loads optional runtime environment files.
package envfile

import (
	"errors"
	"os"

	"github.com/joho/godotenv"
)

// LoadOptional loads path without replacing values already in the process environment.
// A missing file is allowed; other errors are returned to the caller.
func LoadOptional(path string) error {
	if err := godotenv.Load(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	return nil
}
