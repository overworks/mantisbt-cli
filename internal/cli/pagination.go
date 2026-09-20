package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/overworks/mantisbt-cli/internal/mantis"
)

func validateAllPages(fs *flag.FlagSet, all bool, maxPages int64, selection string) error {
	var pageSet, maxPagesSet bool
	fs.Visit(func(f *flag.Flag) {
		pageSet = pageSet || f.Name == "page"
		maxPagesSet = maxPagesSet || f.Name == "max-pages"
	})
	if all && pageSet {
		return fmt.Errorf("--all cannot be combined with --page")
	}
	if maxPagesSet && !all {
		return fmt.Errorf("--max-pages requires --all")
	}
	if maxPages < 1 || maxPages == math.MaxInt64 {
		return fmt.Errorf("--max-pages must be between 1 and %d", int64(math.MaxInt64-1))
	}
	if all && selection != "" {
		for _, field := range strings.Split(selection, ",") {
			if field == "id" {
				return nil
			}
		}
		return fmt.Errorf("--select must include id when using --all to detect repeated issues")
	}
	return nil
}

// listAllIssues waits for an empty, unfiltered page. A server can return fewer
// rows than requested, and a local filter can remove every row on a full page.
// Nothing is printed until the complete traversal succeeds.
func listAllIssues(c *mantis.Client, params map[string]string, status, search string, maxPages int64) (any, error) {
	query := make(map[string]string, len(params))
	for key, value := range params {
		query[key] = value
	}
	issues := make([]any, 0)
	seen := make(map[int64]bool)
	limit := c.MaxResponseBytes
	if limit <= 0 {
		limit = mantis.DefaultMaxResponseBytes
	}
	size := int64(len(`{"issues":[]}`))
	var encodedSize jsonByteCount
	encoder := json.NewEncoder(&encodedSize)
	encoder.SetEscapeHTML(false)
	for page := int64(1); page <= maxPages; page++ {
		query["page"] = strconv.FormatInt(page, 10)
		result, err := c.Get("/api/rest/issues", query)
		if err != nil {
			return nil, fmt.Errorf("issues list: page %d: %w", page, err)
		}
		m, ok := result.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("issues list: page %d: expected an issues array", page)
		}
		items, ok := m["issues"].([]any)
		if !ok {
			return nil, fmt.Errorf("issues list: page %d: expected an issues array", page)
		}
		if len(items) == 0 {
			return map[string]any{"issues": issues}, nil
		}
		for _, item := range items {
			issue, ok := item.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("issues list: page %d: expected an issue object", page)
			}
			id := valueToString(issue["id"])
			idNumber, err := strconv.ParseInt(id, 10, 64)
			if !decimalDigits(id) || err != nil || idNumber < 1 {
				return nil, fmt.Errorf("issues list: page %d: issue is missing a valid id", page)
			}
			if seen[idNumber] {
				return nil, fmt.Errorf("issues list: page %d repeats issue #%d; results changed or pagination did not advance", page, idNumber)
			}
			// IDs from filtered-out issues must also be bounded: they are kept
			// to detect non-advancing pages even when no issues match locally.
			if int64(len(seen)) >= limit/8 {
				return nil, fmt.Errorf("issues list: tracked issue IDs exceed --max-response-size limit; narrow server-side filters or raise the limit")
			}
			seen[idNumber] = true
		}
		if status != "" || search != "" {
			items = filterIssues(m, status, search).(map[string]any)["issues"].([]any)
		}
		for _, item := range items {
			encodedSize = 0
			if err := encoder.Encode(item); err != nil {
				return nil, fmt.Errorf("issues list: page %d: %w", page, err)
			}
			additional := int64(encodedSize) - 1 // Encoder's final newline
			if len(issues) > 0 {
				additional++ // comma between items
			}
			if additional > limit-size {
				return nil, fmt.Errorf("issues list: combined result exceeds --max-response-size limit of %d bytes; narrow the filters or raise the limit", limit)
			}
			size += additional
			issues = append(issues, item)
		}
	}
	return nil, fmt.Errorf("issues list: reached --max-pages limit of %d before an empty page; narrow the filters or raise the limit", maxPages)
}

type jsonByteCount int64

func (c *jsonByteCount) Write(p []byte) (int, error) {
	*c += jsonByteCount(len(p))
	return len(p), nil
}
