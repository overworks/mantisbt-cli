package cli

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"syscall"
	"testing"
	"unsafe"
)

func TestConfirmInteractiveAnswers(t *testing.T) {
	cases := []struct {
		name   string
		answer string
		allow  bool
	}{
		{"yes", "yes\n", true},
		{"short", "y\n", true},
		{"uppercase", "YES\n", true},
		{"whitespace", " yes \n", true},
		{"no", "no\n", false},
		{"default", "\n", false},
		{"other", "anything\n", false},
		{"EOF", "\x04", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			master, input := testTerminal(t)
			if _, err := master.WriteString(tc.answer); err != nil {
				t.Fatal(err)
			}
			var output bytes.Buffer
			err := confirm(input, &output, "Delete? [y/N] ")
			if (err == nil) != tc.allow {
				t.Fatalf("confirmation for %q: err=%v, want allow=%t", tc.answer, err, tc.allow)
			}
			if output.String() != "Delete? [y/N] " {
				t.Fatalf("unexpected prompt: %q", output.String())
			}
		})
	}
}

type failedPromptWriter struct{ err error }

func (w failedPromptWriter) Write([]byte) (int, error) { return 0, w.err }

func TestConfirmRejectsPromptFailure(t *testing.T) {
	_, input := testTerminal(t)
	want := errors.New("output unavailable")
	if err := confirm(input, failedPromptWriter{want}, "Delete? "); !errors.Is(err, want) {
		t.Fatalf("expected prompt failure, got %v", err)
	}
}

// testTerminal opens a Linux pseudo-terminal so terminal detection and actual
// interactive input are tested together without requiring a developer's tty.
func testTerminal(t *testing.T) (*os.File, *os.File) {
	t.Helper()
	master, err := os.OpenFile("/dev/ptmx", os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		t.Skipf("pseudo-terminals unavailable: %v", err)
	}
	t.Cleanup(func() { master.Close() })
	var unlock int32
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, master.Fd(), syscall.TIOCSPTLCK, uintptr(unsafe.Pointer(&unlock))); errno != 0 {
		t.Fatal(errno)
	}
	var number uint32
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, master.Fd(), syscall.TIOCGPTN, uintptr(unsafe.Pointer(&number))); errno != 0 {
		t.Fatal(errno)
	}
	input, err := os.OpenFile(fmt.Sprintf("/dev/pts/%d", number), os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { input.Close() })
	return master, input
}
