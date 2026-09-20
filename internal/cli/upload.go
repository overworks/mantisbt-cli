package cli

import (
	"fmt"
	"io"
	"os"
)

const defaultMaxUploadBytes int64 = 32 << 20

func readUpload(path string, remaining int64) ([]byte, error) {
	// Check before opening so ordinary FIFOs and devices are rejected without
	// blocking. Check the opened file again before allocating for its contents.
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("only regular files can be uploaded")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err = file.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("only regular files can be uploaded")
	}
	if info.Size() > remaining {
		return nil, fmt.Errorf("combined files exceed --max-upload-size (%d bytes remaining)", remaining)
	}
	data, err := io.ReadAll(io.LimitReader(file, remaining+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > remaining {
		return nil, fmt.Errorf("combined files exceed --max-upload-size (%d bytes remaining)", remaining)
	}
	return data, nil
}
