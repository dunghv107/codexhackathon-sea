package query

import (
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"strings"
	"testing"
)

func TestPaginationAndParsing(t *testing.T) {
	for _, tc := range []struct {
		p        map[string]string
		off, lim int
	}{{nil, 0, 20}, {map[string]string{"page": "3", "limit": "5"}, 10, 5}, {map[string]string{"offset": "8", "limit": "999"}, 8, 100}, {map[string]string{"page": "bad", "limit": "-1"}, 0, 20}, {map[string]string{"offset": "bad", "limit": "bad"}, 0, 20}} {
		o, l := GetPagination(tc.p, 20)
		if o != tc.off || l != tc.lim {
			t.Fatal(tc, o, l)
		}
	}
	opts := ParseQueryParams(map[string]string{"sort": "name, -id, ,password", "name[like]": "test", "ignored": "no", "id": "", "name]oops[": "bad"}, map[string]bool{"name": true, "id": true})
	if len(opts.Filters) != 1 || opts.Filters["name"].Operator != "like" || len(opts.Sort) != 3 || !opts.Sort[1].Desc {
		t.Fatal(opts)
	}
}
func TestScopesUseAllowedIdentifiersAndBoundValues(t *testing.T) {
	db, e := gorm.Open(postgres.New(postgres.Config{DSN: "host=unused", PreferSimpleProtocol: true}), &gorm.Config{DryRun: true, DisableAutomaticPing: true})
	if e != nil {
		t.Fatal(e)
	}
	for _, op := range []string{"eq", "ne", "gt", "gte", "lt", "lte", "like", "in", "nin", "unknown"} {
		opts := NewQueryOptions()
		var value any = "x' OR 1=1 --"
		if op == "in" || op == "nin" {
			value = []string{"safe", "other"}
		}
		opts.AddFilter("name", op, value)
		opts.AddFilter("ignored", "eq", "bad")
		opts.AddFilter("bad;DROP", "eq", "bad")
		opts.AddSort("name", false)
		opts.AddSort("id", true)
		opts.AddSort("ignored", false)
		opts.AddSort("bad;DROP", false)
		result := db.Table("users").Scopes(ApplyFilters(opts, map[string]bool{"name": true, "bad;DROP": true}), ApplySort(opts, map[string]bool{"name": true, "id": true, "bad;DROP": true}), ApplyDefaultSort(opts, "id", true)).Find(&[]map[string]any{})
		sql := result.Statement.SQL.String()
		if strings.Contains(sql, "DROP") || strings.Contains(sql, "OR 1=1") || !strings.Contains(sql, "name ASC,id DESC") {
			t.Fatal(sql)
		}
		if op != "unknown" && len(result.Statement.Vars) == 0 {
			t.Fatalf("unbound filter %s", op)
		}
	}
	for _, desc := range []bool{true, false} {
		r := db.Table("users").Scopes(ApplyDefaultSort(NewQueryOptions(), "id", desc)).Find(&[]map[string]any{})
		if !strings.Contains(r.Statement.SQL.String(), "ORDER BY id") {
			t.Fatal(r.Statement.SQL.String())
		}
	}
}
