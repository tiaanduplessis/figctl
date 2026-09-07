package figma

import (
	"net/url"
	"strings"
)

// SplitIDs splits ids into batches whose comma-joined, URL-encoded form
// stays under maxBytes. A single id longer than the budget gets its own
// batch. Order is preserved and duplicates are dropped.
func SplitIDs(ids []string, maxBytes int) [][]string {
	if len(ids) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(ids))
	var batches [][]string
	var current []string
	size := 0
	for _, id := range ids {
		if seen[id] {
			continue
		}
		seen[id] = true
		encoded := len(url.QueryEscape(id))
		next := size + encoded
		if len(current) > 0 {
			next += len(url.QueryEscape(","))
		}
		if len(current) > 0 && next > maxBytes {
			batches = append(batches, current)
			current = nil
			next = encoded
		}
		current = append(current, id)
		size = next
	}
	if len(current) > 0 {
		batches = append(batches, current)
	}
	return batches
}

// idBudget returns the byte budget for the ids parameter of a request so
// that the full URL stays under maxURLBytes.
func (c *Client) idBudget(path string, other url.Values) int {
	fixed := len(c.baseURL) + len(path) + len("?ids=")
	if len(other) > 0 {
		fixed += len(other.Encode()) + 1
	}
	budget := maxURLBytes - fixed
	if budget < 256 {
		budget = 256
	}
	return budget
}

func joinIDs(ids []string) string {
	return strings.Join(ids, ",")
}
