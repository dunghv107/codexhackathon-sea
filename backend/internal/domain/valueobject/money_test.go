package valueobject

import "testing"

func TestParseCents(t *testing.T) {
	ok := map[string]int64{"0": 0, "129.90": 12990, "129.9": 12990, "5": 500, "0.01": 1, " 10.00 ": 1000}
	for in, want := range ok {
		got, err := ParseCents(in)
		if err != nil || got != want {
			t.Errorf("ParseCents(%q) = %d, %v; want %d", in, got, err, want)
		}
	}
	for _, in := range []string{"", ".5", "1.", "1.234", "-1", "1e3", "abc", "1,50", "99999999999999999999"} {
		if _, err := ParseCents(in); err == nil {
			t.Errorf("ParseCents(%q) accepted, want error", in)
		}
	}
}

func TestFormatCentsAndSum(t *testing.T) {
	cases := map[int64]string{0: "0.00", 1: "0.01", 12990: "129.90", 100000: "1000.00", -250: "-2.50"}
	for in, want := range cases {
		if got := FormatCents(in); got != want {
			t.Errorf("FormatCents(%d) = %q, want %q", in, got, want)
		}
	}

	// 0.10 + 0.20 must be exactly 0.30 (classic float trap).
	var sum int64
	for _, s := range []string{"0.10", "0.20"} {
		c, _ := ParseCents(s)
		sum += c
	}
	if got := FormatCents(sum); got != "0.30" {
		t.Fatalf("sum = %q, want 0.30", got)
	}
}

func TestAddLineCents(t *testing.T) {
	if got, err := AddLineCents(100, 12990, 3); err != nil || got != 39070 {
		t.Fatalf("AddLineCents = %d, %v; want 39070", got, err)
	}
	if got, err := AddLineCents(0, maxCents, 1); err != nil || got != maxCents {
		t.Fatalf("AddLineCents at limit = %d, %v", got, err)
	}
	// 99 * 931653741096442.01 wraps int64 if multiplied blindly.
	for _, c := range [][3]int64{{0, 93165374109644201, 99}, {1, maxCents, 1}, {0, 1, 0}} {
		if _, err := AddLineCents(c[0], c[1], int(c[2])); err == nil {
			t.Errorf("AddLineCents(%d, %d, %d) accepted, want error", c[0], c[1], c[2])
		}
	}
}
