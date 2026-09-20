package cli

import (
	"errors"
	"flag"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
)

var errUsage = errors.New("invalid command arguments")

func validByteLimit(command, name string, size int64) bool {
	if size > 0 && size < math.MaxInt64 {
		return true
	}
	fmt.Fprintf(os.Stderr, "%s: %s must be between 1 and %d bytes\n", command, name, int64(math.MaxInt64-1))
	return false
}

func flagExitCode(err error) int {
	if errors.Is(err, flag.ErrHelp) {
		return 0
	}
	return 2
}

func isHelpFlag(value string) bool { return value == "-h" || value == "--help" }

func commandFlags(name, positionals string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "usage: mantisbt-cli %s", name)
		if positionals != "" {
			fmt.Fprintf(fs.Output(), " %s", positionals)
		}
		hasFlags := false
		fs.VisitAll(func(*flag.Flag) { hasFlags = true })
		if hasFlags {
			fmt.Fprint(fs.Output(), " [flags]")
		}
		fmt.Fprintln(fs.Output())
		fs.PrintDefaults()
	}
	return fs
}

// parseCommandFlags keeps IDs before flags while allowing help before all IDs
// have been supplied. Help appearing as a flag value is left to flag.Parse.
func parseCommandFlags(fs *flag.FlagSet, args []string, idCount int) ([]string, error) {
	for i := 0; i < idCount && i < len(args); i++ {
		if isHelpFlag(args[i]) {
			fs.Usage()
			return nil, flag.ErrHelp
		}
	}
	if len(args) < idCount {
		fs.Usage()
		return nil, errUsage
	}
	if err := fs.Parse(args[idCount:]); err != nil {
		return nil, err
	}
	return args[:idCount], nil
}

func hasUnexpectedArgs(fs *flag.FlagSet) bool {
	if fs.NArg() == 0 {
		return false
	}
	fmt.Fprintf(os.Stderr, "%s: unexpected argument %q\n", fs.Name(), fs.Arg(0))
	return true
}

func decimalDigits(value string) bool {
	if value == "" {
		return false
	}
	for _, ch := range value {
		if ch < '0' || ch > '9' {
			return false
		}
	}
	return true
}

func validInteger(command, name, value string, minimum int64) bool {
	number, err := strconv.ParseInt(value, 10, 64)
	if decimalDigits(value) && err == nil && number >= minimum {
		return true
	}
	kind := "positive"
	if minimum == 0 {
		kind = "non-negative"
	}
	fmt.Fprintf(os.Stderr, "%s: %s must be a %s decimal integer (got %q)\n", command, name, kind, value)
	return false
}

func validProject(value string) bool {
	// Numeric-looking values are project ids, including invalid signed ids;
	// all other values retain the documented project-name behavior.
	digits := strings.TrimPrefix(strings.TrimPrefix(value, "+"), "-")
	if decimalDigits(digits) {
		return validInteger("issue create", "--project", value, 1)
	}
	return true
}

func validFilter(value string) bool {
	switch value {
	case "", "assigned", "reported", "monitored", "unassigned":
		return true
	}
	if decimalDigits(value) {
		return validInteger("issues list", "--filter", value, 1)
	}
	fmt.Fprintln(os.Stderr, "issues list: --filter must be assigned, reported, monitored, unassigned, or a positive stored filter id")
	return false
}

// validateSelection keeps the requested response fields explicit while ensuring
// local filters have the data they need. An empty selection uses API defaults.
func validateSelection(selection, status, search string) (string, error) {
	if selection == "" {
		return "", nil
	}
	fields := strings.Split(selection, ",")
	selected := make(map[string]bool, len(fields))
	for i, field := range fields {
		field = strings.TrimSpace(field)
		if field == "" {
			return "", fmt.Errorf("--select must contain non-empty, comma-separated field names")
		}
		fields[i] = field
		selected[field] = true
	}
	if status != "" && !selected["status"] {
		return "", fmt.Errorf("--select must include status when using --status")
	}
	if search != "" && !selected["summary"] {
		return "", fmt.Errorf("--select must include summary when using --search")
	}
	return strings.Join(fields, ","), nil
}
