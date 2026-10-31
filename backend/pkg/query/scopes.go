package query

import (
	"fmt"
	"regexp"

	"gorm.io/gorm"
)

// validFieldName validates that a field name contains only safe characters
var validFieldName = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

// ApplyFilters returns a GORM scope that applies all filters dynamically
func ApplyFilters(opts QueryOptions, allowedFields map[string]bool) func(*gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		for field, filter := range opts.Filters {
			if !allowedFields[field] {
				continue
			}
			if !validFieldName.MatchString(field) {
				continue
			}
			db = applyFilter(db, field, filter)
		}
		return db
	}
}

// ApplySort returns a GORM scope that applies all sort fields dynamically
func ApplySort(opts QueryOptions, allowedFields map[string]bool) func(*gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		for _, sort := range opts.Sort {
			if !allowedFields[sort.Field] {
				continue
			}
			if !validFieldName.MatchString(sort.Field) {
				continue
			}
			direction := "ASC"
			if sort.Desc {
				direction = "DESC"
			}
			db = db.Order(fmt.Sprintf("%s %s", sort.Field, direction))
		}
		return db
	}
}

// ApplyDefaultSort returns a GORM scope that applies default sort if no sort specified
func ApplyDefaultSort(opts QueryOptions, defaultField string, defaultDesc bool) func(*gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		if len(opts.Sort) == 0 {
			direction := "ASC"
			if defaultDesc {
				direction = "DESC"
			}
			db = db.Order(fmt.Sprintf("%s %s", defaultField, direction))
		}
		return db
	}
}

// applyFilter applies a single filter to the query based on operator
func applyFilter(db *gorm.DB, field string, filter FilterValue) *gorm.DB {
	switch filter.Operator {
	case "eq":
		return db.Where(fmt.Sprintf("%s = ?", field), filter.Value)
	case "ne":
		return db.Where(fmt.Sprintf("%s != ?", field), filter.Value)
	case "gt":
		return db.Where(fmt.Sprintf("%s > ?", field), filter.Value)
	case "gte":
		return db.Where(fmt.Sprintf("%s >= ?", field), filter.Value)
	case "lt":
		return db.Where(fmt.Sprintf("%s < ?", field), filter.Value)
	case "lte":
		return db.Where(fmt.Sprintf("%s <= ?", field), filter.Value)
	case "like":
		return db.Where(fmt.Sprintf("%s ILIKE ?", field), fmt.Sprintf("%%%v%%", filter.Value))
	case "in":
		return db.Where(fmt.Sprintf("%s IN ?", field), filter.Value)
	case "nin":
		return db.Where(fmt.Sprintf("%s NOT IN ?", field), filter.Value)
	default:
		return db
	}
}
