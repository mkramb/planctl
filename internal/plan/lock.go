package plan

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/gofrs/flock"
)

func lockWorkspace(root, slug string) (*flock.Flock, error) {
	locks := filepath.Join(root, ".locks")
	if err := makeDirectories(locks); err != nil {
		return nil, err
	}
	path := filepath.Join(locks, slug+".lock")
	if info, err := os.Lstat(path); err == nil && !info.Mode().IsRegular() {
		return nil, &Error{Code: "workspace_conflict", Message: "plan lock path is not a regular file"}
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	lock := flock.New(path)
	locked, err := lock.TryLock()
	if err != nil {
		return nil, fmt.Errorf("lock plan workspace: %w", err)
	}
	if !locked {
		return nil, &Error{Code: "plan_busy", Message: "another command is modifying this plan; try again when it finishes"}
	}
	return lock, nil
}
