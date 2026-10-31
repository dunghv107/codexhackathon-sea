package query

// QueryOptions holds dynamic filter and sort options for list queries
type QueryOptions struct {
	Filters map[string]FilterValue
	Sort    []SortField
}

// FilterValue represents a filter with operator and value
type FilterValue struct {
	Operator string // eq, ne, gt, gte, lt, lte, like, in, nin
	Value    any
}

// SortField represents a single sort criterion
type SortField struct {
	Field string
	Desc  bool
}

// NewQueryOptions creates a new QueryOptions instance
func NewQueryOptions() QueryOptions {
	return QueryOptions{
		Filters: make(map[string]FilterValue),
		Sort:    []SortField{},
	}
}

// AddFilter adds a filter to the query options
func (q *QueryOptions) AddFilter(field, operator string, value any) {
	q.Filters[field] = FilterValue{
		Operator: operator,
		Value:    value,
	}
}

// AddSort adds a sort field to the query options
func (q *QueryOptions) AddSort(field string, desc bool) {
	q.Sort = append(q.Sort, SortField{
		Field: field,
		Desc:  desc,
	})
}
