package apiclient

import (
	"net/url"
	"strconv"
)

// PageOptions configures pagination for list requests.
type PageOptions struct {
	Limit  int    // Maximum results per page (default varies by endpoint)
	Cursor string // Pagination cursor from previous response
}

// PageResult contains pagination metadata from a list response.
type PageResult struct {
	NextCursor string // Cursor for the next page (empty if no more pages)
	TotalCount int    // Total count of items (if available)
}

// HasMore returns true if there are more pages available.
func (p *PageResult) HasMore() bool {
	return p.NextCursor != ""
}

// ToQuery adds pagination parameters to a URL query.
func (p *PageOptions) ToQuery(q url.Values) url.Values {
	if q == nil {
		q = url.Values{}
	}
	if p.Limit > 0 {
		q.Set("limit", strconv.Itoa(p.Limit))
	}
	if p.Cursor != "" {
		q.Set("cursor", p.Cursor)
	}
	return q
}
