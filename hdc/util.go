package hdc

import (
	"regexp"
	"strings"
)

var reKeyVal = regexp.MustCompile(`^\s*(.*?) = (.*?)\r?$`)

type Forward struct {
	Target string
	Local  string
	Remote string
}

func readTargets(s string) []string {
	if strings.Contains(s, "Empty") {
		return []string{}
	}
	lines := strings.Split(s, "\n")
	out := make([]string, 0, len(lines))
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if l != "" {
			out = append(out, l)
		}
	}
	return out
}

func readPorts(s string, reverse bool) []Forward {
	if strings.Contains(s, "Empty") {
		return []Forward{}
	}
	lines := strings.Split(s, "\n")
	out := make([]Forward, 0)
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if l == "" {
			continue
		}
		if !strings.Contains(l, map[bool]string{false: "Forward", true: "Reverse"}[reverse]) {
			continue
		}
		parts := strings.Fields(l)
		if len(parts) < 3 {
			continue
		}
		if reverse {
			out = append(out, Forward{Target: parts[0], Local: parts[2], Remote: parts[1]})
		} else {
			out = append(out, Forward{Target: parts[0], Local: parts[1], Remote: parts[2]})
		}
	}
	return out
}

func parseParameters(s string) map[string]string {
	res := map[string]string{}
	lines := strings.Split(s, "\n")
	for _, l := range lines {
		if m := reKeyVal.FindStringSubmatch(l); len(m) == 3 {
			res[m[1]] = m[2]
		}
	}
	return res
}
