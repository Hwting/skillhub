package skill

import "testing"

func TestIsValid(t *testing.T) {
	good := []string{"1.0.0", "0.0.1", "1.2.3-alpha", "1.2.3-alpha.1", "1.0.0+build", "1.0.0-beta+x"}
	bad := []string{"", "1", "1.0", "v1.0.0", "1.0.0.0", "01.0.0", "a.b.c", "1.0.0-"}
	for _, v := range good {
		if !IsValid(v) {
			t.Fatalf("expected valid: %s", v)
		}
	}
	for _, v := range bad {
		if IsValid(v) {
			t.Fatalf("expected invalid: %s", v)
		}
	}
}

func TestCompare(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"1.0.0", "1.0.0", 0},
		{"1.0.0", "2.0.0", -1},
		{"2.0.0", "1.0.0", 1},
		{"1.0.0", "1.0.1", -1},
		{"1.0.0", "1.0.0-alpha", 1}, // prerelease < release
		{"1.0.0-alpha", "1.0.0-beta", -1},
		{"1.0.0+build1", "1.0.0+build2", 0}, // build ignored
	}
	for _, c := range cases {
		if got := Compare(c.a, c.b); got != c.want {
			t.Fatalf("Compare(%s,%s)=%d want %d", c.a, c.b, got, c.want)
		}
	}
}
