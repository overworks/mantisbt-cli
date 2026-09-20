package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// attachmentPath limits a server-supplied filename to the current directory.
// Existence checks belong to writeAttachment so checking and creating a default
// destination cannot race with another download.
func attachmentPath(output, filename, fileID string) (string, error) {
	if output != "" {
		return output, nil
	}
	name := filepath.Base(filename)
	if name == "." || !filepath.IsLocal(name) {
		return "", fmt.Errorf("attachment #%s has no usable filename; pass --output", fileID)
	}
	return name, nil
}

// writeAttachment creates private files. A default destination must not exist,
// including as a dangling symlink. Explicit --output replaces the destination
// only after writing and closing a private temporary file in the same directory;
// this also replaces a symlink itself rather than following its target.
func writeAttachment(path string, data io.Reader, overwrite bool) (int64, error) {
	var file *os.File
	var err error
	if overwrite {
		file, err = os.CreateTemp(filepath.Dir(path), ".mantisbt-cli-download-*")
	} else {
		file, err = os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	}
	if err != nil {
		if !overwrite && errors.Is(err, os.ErrExist) {
			return 0, fmt.Errorf("%s already exists; pass --output to choose a destination", path)
		}
		return 0, fmt.Errorf("create %s: %w", path, err)
	}

	writtenPath := file.Name()
	keep := false
	defer func() {
		if !keep {
			os.Remove(writtenPath)
		}
	}()
	size, writeErr := io.Copy(file, data)
	closeErr := file.Close()
	if err := errors.Join(writeErr, closeErr); err != nil {
		return 0, fmt.Errorf("write %s: %w", path, err)
	}
	if overwrite {
		if err := os.Rename(writtenPath, path); err != nil {
			return 0, fmt.Errorf("replace %s: %w", path, err)
		}
	}
	keep = true
	return size, nil
}
