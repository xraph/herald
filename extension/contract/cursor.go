package contract

import (
	"encoding/base64"
	"strconv"
	"strings"
)

// Cursors are opaque to the client and wrap an offset. Lists that grow
// (messages, inbox) page with them; the handler asks the store for limit+1
// rows to know whether there is a next page, and never computes a total.

func encodeCursor(offset int) string {
	return base64.RawURLEncoding.EncodeToString([]byte("o:" + strconv.Itoa(offset)))
}

func decodeCursor(s string) (int, error) {
	if s == "" {
		return 0, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return 0, badRequest("cursor is not valid")
	}
	n, ok := strings.CutPrefix(string(raw), "o:")
	if !ok {
		return 0, badRequest("cursor is not valid")
	}
	offset, err := strconv.Atoi(n)
	if err != nil || offset < 0 {
		return 0, badRequest("cursor is not valid")
	}
	return offset, nil
}

// pageLimit applies a default to a missing or non-positive limit and caps a
// large one. A page size is a preference, so an oversized one is capped, not
// refused.
func pageLimit(requested, def, maxLimit int) int {
	if requested <= 0 {
		return def
	}
	if requested > maxLimit {
		return maxLimit
	}
	return requested
}

// nextCursor returns the cursor for the page after one that started at offset
// and asked for limit rows, given how many rows came back from a limit+1
// read. It returns "" when there is no next page.
func nextCursor(offset, limit, got int) string {
	if got <= limit {
		return ""
	}
	return encodeCursor(offset + limit)
}
