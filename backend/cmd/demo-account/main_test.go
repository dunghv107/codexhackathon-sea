package main

import "testing"

func TestValidate(t *testing.T) {
	for _, c := range []struct {
		username, password, buyer, shops string
		valid                            bool
	}{
		{"reviewer", "password123", "", "sim:shop", true},
		{"buyer", "password123", "sim:buyer", "", true},
		{"reviewer", "short", "", "sim:shop", false},
		{"reviewer", "password123", "", "", false},
		{"reviewer", "password123", "", "sim:shop,", false},
		{"bad name", "password123", "sim:buyer", "", false},
	} {
		if (validate(c.username, c.password, c.buyer, c.shops) == nil) != c.valid {
			t.Errorf("unexpected validation result for %q", c.username)
		}
	}
}
