package skill

import (
	"regexp"
	"strconv"
	"strings"
)

var semverRe = regexp.MustCompile(`^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(-[a-zA-Z0-9.]+)?(\+[a-zA-Z0-9.]+)?$`)

// IsValid reports whether v is a syntactically valid semver (npm-style,
// no leading "v"): MAJOR.MINOR.PATCH with optional -prerelease and +build.
func IsValid(v string) bool { return semverRe.MatchString(v) }

// Compare returns -1/0/1. Orders by major.minor.patch; a prerelease is
// less than the corresponding release; build metadata is ignored.
// Invalid versions fall back to lexicographic order.
func Compare(a, b string) int {
	pa, oka := parseSemver(a)
	pb, okb := parseSemver(b)
	if !oka || !okb {
		return strings.Compare(a, b)
	}
	for i := 0; i < 3; i++ {
		if pa.n[i] != pb.n[i] {
			if pa.n[i] < pb.n[i] {
				return -1
			}
			return 1
		}
	}
	switch {
	case pa.pre == "" && pb.pre != "":
		return 1
	case pa.pre != "" && pb.pre == "":
		return -1
	default:
		return strings.Compare(pa.pre, pb.pre)
	}
}

type semverParts struct {
	n   [3]int
	pre string
}

func parseSemver(v string) (semverParts, bool) {
	if !IsValid(v) {
		return semverParts{}, false
	}
	if build := strings.Index(v, "+"); build >= 0 {
		v = v[:build]
	}
	pre := ""
	if p := strings.Index(v, "-"); p >= 0 {
		pre = v[p+1:]
		v = v[:p]
	}
	parts := strings.Split(v, ".")
	out := semverParts{pre: pre}
	for i, s := range parts {
		n, _ := strconv.Atoi(s)
		out.n[i] = n
	}
	return out, true
}
