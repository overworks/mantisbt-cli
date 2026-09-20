package cli

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestIssueFileGetEmptyAndMissingContent(t *testing.T) {
	for _, content := range []string{`"content":""`, `"content":null`, `"content":123`, `"other":""`} {
		t.Run(content, func(t *testing.T) {
			dest := filepath.Join(t.TempDir(), "empty.txt")
			call, _ := route([]string{"issue", "file", "get", "123", "5", "--output", dest})
			client, _, _ := recordingServer(t, `{"files":[{"id":5,"filename":"empty.txt","size":0,`+content+`}]}`)
			_, err := call(client)
			if content == `"content":""` {
				if err != nil {
					t.Fatal(err)
				}
				assertFileContent(t, dest, "")
			} else if err == nil {
				t.Fatal("missing/non-string content must fail")
			}
		})
	}
}

func TestIssueFileGetRejectsEmptyContentForNonEmptyAttachment(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "original.txt")
	if err := os.WriteFile(dest, []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}
	call, _ := route([]string{"issue", "file", "get", "123", "5", "--output", dest})
	client, _, _ := recordingServer(t, `{"files":[{"id":5,"filename":"file.txt","size":3,"content":""}]}`)
	if _, err := call(client); err == nil {
		t.Fatal("empty content cannot represent a non-empty attachment")
	}
	assertFileContent(t, dest, "original")
}

type failedDownloadReader struct{}

func (failedDownloadReader) Read([]byte) (int, error) { return 0, errors.New("download interrupted") }

func TestWriteAttachmentCleansUpPartialStream(t *testing.T) {
	for _, overwrite := range []bool{false, true} {
		dir := t.TempDir()
		dest := filepath.Join(dir, "file")
		if overwrite {
			if err := os.WriteFile(dest, []byte("original"), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		reader := io.MultiReader(strings.NewReader("partial data"), failedDownloadReader{})
		if _, err := writeAttachment(dest, reader, overwrite); err == nil {
			t.Fatal("stream error must fail")
		}
		wantEntries := 0
		if overwrite {
			wantEntries = 1
			assertFileContent(t, dest, "original")
		}
		entries, err := os.ReadDir(dir)
		if err != nil || len(entries) != wantEntries {
			t.Fatalf("partial files remain: %v err=%v", entries, err)
		}
	}
}

func TestIssueFileGetRejectsExistingDefaultDestination(t *testing.T) {
	for _, kind := range []string{"file", "symlink", "dangling symlink"} {
		t.Run(kind, func(t *testing.T) {
			dir := t.TempDir()
			t.Chdir(dir)
			target := filepath.Join(t.TempDir(), "outside.txt")
			if kind != "dangling symlink" {
				if err := os.WriteFile(target, []byte("original"), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if kind == "file" {
				if err := os.WriteFile("download.txt", []byte("original"), 0o644); err != nil {
					t.Fatal(err)
				}
			} else {
				symlinkForTest(t, target, "download.txt")
			}

			call, _ := route([]string{"issue", "file", "get", "123", "5"})
			client, _, _ := recordingServer(t, `{"files":[{"id":5,"filename":"download.txt","content":"bmV3"}]}`)
			if _, err := call(client); err == nil || !strings.Contains(err.Error(), "already exists") {
				t.Fatalf("expected existing destination to be rejected, got %v", err)
			}
			if kind == "dangling symlink" {
				if _, err := os.Lstat(target); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("dangling symlink target was created: %v", err)
				}
			} else {
				assertFileContent(t, "download.txt", "original")
				assertFileContent(t, target, "original")
			}
			if kind != "file" {
				if got, err := os.Readlink("download.txt"); err != nil || got != target {
					t.Fatalf("symlink was changed: target=%q, err=%v", got, err)
				}
			}
		})
	}
}

func TestIssueFileGetReplacesExplicitOutputPrivately(t *testing.T) {
	for _, kind := range []string{"file", "symlink", "dangling symlink"} {
		t.Run(kind, func(t *testing.T) {
			dir := t.TempDir()
			dest := filepath.Join(dir, "download.txt")
			target := filepath.Join(t.TempDir(), "outside.txt")
			if kind == "file" {
				if err := os.WriteFile(dest, []byte("original"), 0o644); err != nil {
					t.Fatal(err)
				}
			} else {
				if kind == "symlink" {
					if err := os.WriteFile(target, []byte("original"), 0o644); err != nil {
						t.Fatal(err)
					}
				}
				symlinkForTest(t, target, dest)
			}

			call, _ := route([]string{"issue", "file", "get", "123", "5", "--output", dest})
			client, _, _ := recordingServer(t, `{"files":[{"id":5,"filename":"server.txt","content":"bmV3"}]}`)
			if _, err := call(client); err != nil {
				t.Fatal(err)
			}
			assertFileContent(t, dest, "new")
			info, err := os.Lstat(dest)
			if err != nil {
				t.Fatal(err)
			}
			if !info.Mode().IsRegular() {
				t.Fatalf("output must be a regular file, got %v", info.Mode())
			}
			if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
				t.Fatalf("expected private permissions after replacement, got %04o", info.Mode().Perm())
			}
			if kind == "symlink" {
				assertFileContent(t, target, "original")
			} else if kind == "dangling symlink" {
				if _, err := os.Lstat(target); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("symlink target was created: %v", err)
				}
			}
			entries, err := os.ReadDir(dir)
			if err != nil || len(entries) != 1 {
				t.Fatalf("unexpected temporary files after download: entries=%v, err=%v", entries, err)
			}
		})
	}
}

func TestIssueFileGetKeepsOutputOnMalformedContent(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "download.txt")
	if err := os.WriteFile(dest, []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}
	call, _ := route([]string{"issue", "file", "get", "123", "5", "--output", dest})
	client, _, _ := recordingServer(t, `{"files":[{"id":5,"filename":"server.txt","content":"not base64!"}]}`)
	if _, err := call(client); err == nil {
		t.Fatal("malformed attachment must fail")
	}
	assertFileContent(t, dest, "original")
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 {
		t.Fatalf("unexpected temporary files after failure: entries=%v, err=%v", entries, err)
	}
}

func TestWriteAttachmentRejectsFileCreatedAfterPathSelection(t *testing.T) {
	t.Chdir(t.TempDir())
	path, err := attachmentPath("", "download.txt", "5")
	if err != nil {
		t.Fatal(err)
	}
	// Another process creates the file between choosing its name and writing.
	if err := os.WriteFile(path, []byte("other download"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := writeAttachment(path, strings.NewReader("new"), false); err == nil {
		t.Fatal("must not overwrite a file created after path selection")
	}
	assertFileContent(t, path, "other download")
}

func TestWriteAttachmentCleansUpFailedReplacement(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "existing-directory")
	if err := os.Mkdir(dest, 0o700); err != nil {
		t.Fatal(err)
	}
	sentinel := filepath.Join(dest, "original.txt")
	if err := os.WriteFile(sentinel, []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := writeAttachment(dest, strings.NewReader("new"), true); err == nil {
		t.Fatal("replacing a directory must fail")
	}
	assertFileContent(t, sentinel, "original")
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 {
		t.Fatalf("temporary file was not cleaned up: entries=%v, err=%v", entries, err)
	}
}

func TestIssueFileGetCreatesPrivateDefaultDestination(t *testing.T) {
	t.Chdir(t.TempDir())
	call, _ := route([]string{"issue", "file", "get", "123", "5"})
	client, _, _ := recordingServer(t, `{"files":[{"id":5,"filename":"../../download.txt","content":"bmV3"}]}`)
	result, err := call(client)
	if err != nil {
		t.Fatal(err)
	}
	assertFileContent(t, "download.txt", "new")
	info, err := os.Stat("download.txt")
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatalf("expected private permissions, got %04o", info.Mode().Perm())
	}
	download := result.(downloaded)
	if download.path != "download.txt" || download.size != 3 {
		t.Fatalf("unexpected result: %+v", download)
	}
}

func TestAttachmentPathRejectsUnsafeNames(t *testing.T) {
	names := []string{"", ".", "..", string(filepath.Separator)}
	if runtime.GOOS == "windows" {
		names = append(names, "NUL", "CON", "file:stream")
	}
	for _, name := range names {
		if _, err := attachmentPath("", name, "5"); err == nil {
			t.Errorf("expected %q to be rejected", name)
		}
	}
}

func symlinkForTest(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		if runtime.GOOS == "windows" {
			t.Skipf("symlinks unavailable: %v", err)
		}
		t.Fatal(err)
	}
}

func assertFileContent(t *testing.T, path, want string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil || string(data) != want {
		t.Fatalf("%s: content=%q, err=%v; want %q", path, data, err, want)
	}
}
