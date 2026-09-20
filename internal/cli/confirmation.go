package cli

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
)

// confirm accepts confirmation only from a terminal. Piped or redirected input
// cannot authorize a deletion, even if it contains "yes".
func confirm(input *os.File, output io.Writer, prompt string) error {
	if !isTerminal(input.Fd()) {
		return fmt.Errorf("confirmation requires an interactive terminal; pass --yes to confirm deletion")
	}
	if _, err := fmt.Fprint(output, prompt); err != nil {
		return fmt.Errorf("confirmation prompt: %w", err)
	}
	line, err := bufio.NewReader(input).ReadString('\n')
	if err != nil {
		return fmt.Errorf("aborted")
	}
	answer := strings.ToLower(strings.TrimSpace(line))
	if answer != "y" && answer != "yes" {
		return fmt.Errorf("aborted")
	}
	return nil
}
