package query

import (
	"strconv"
	"strings"
)

// parseOperator parses operator from query parameter format: field[op]=value
// e.g., status[eq]=active, name[like]=john
func parseOperator(key string) (field, operator string) {
	start := strings.Index(key, "[")
	end := strings.Index(key, "]")

	if start == -1 || end == -1 || end < start {
		return key, "eq" // Default to eq
	}

	field = key[:start]
	operator = key[start+1 : end]
	return field, operator
}

// ParseQueryParams parses query parameters into QueryOptions
func ParseQueryParams(params map[string]string, allowedFields map[string]bool) QueryOptions {
	opts := NewQueryOptions()

	for key, value := range params {
		if value == "" {
			continue
		}

		// Handle sort parameter
		if key == "sort" {
			parseSortParam(value, &opts)
			continue
		}

		// Handle filter parameters
		field, operator := parseOperator(key)
		if !allowedFields[field] {
			continue
		}

		opts.AddFilter(field, operator, value)
	}

	return opts
}

// parseSortParam parses sort parameter: sort=field1,-field2 (- prefix for DESC)
func parseSortParam(value string, opts *QueryOptions) {
	fields := strings.Split(value, ",")
	for _, f := range fields {
		f = strings.TrimSpace(f)
		if f == "" {
			continue
		}

		desc := false
		if strings.HasPrefix(f, "-") {
			desc = true
			f = f[1:]
		}

		opts.AddSort(f, desc)
	}
}

// GetPagination extracts offset and limit from query params
func GetPagination(params map[string]string, defaultLimit int) (offset, limit int) {
	limit = defaultLimit

	if l, ok := params["limit"]; ok {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
			if limit > 100 {
				limit = 100
			}
		}
	}

	if p, ok := params["page"]; ok {
		if page, err := strconv.Atoi(p); err == nil && page > 0 {
			offset = (page - 1) * limit
		}
	} else if o, ok := params["offset"]; ok {
		if parsed, err := strconv.Atoi(o); err == nil && parsed >= 0 {
			offset = parsed
		}
	}

	return offset, limit
}
