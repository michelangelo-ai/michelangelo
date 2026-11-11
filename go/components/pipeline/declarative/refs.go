package declarative

import (
	"fmt"
	"regexp"
	"strings"
)

type refKind int

const (
	refParam refKind = iota
	refTaskOutput
)

type ref struct {
	Kind  refKind
	Name  string
	Path  []string
	Whole bool
}

var (
	refName  = regexp.MustCompile(`^[a-z][a-z0-9_]{0,62}$`)
	refField = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
)

func parseRefs(s string) ([]ref, error) {
	var out []ref
	rest := s
	for {
		start := strings.Index(rest, "{{")
		if start < 0 {
			return out, nil
		}
		end := strings.Index(rest[start:], "}}")
		if end < 0 {
			return nil, fmt.Errorf("unterminated \"{{\" in %q", s)
		}
		token := rest[start : start+end+2]
		r, err := parseOneRef(token)
		if err != nil {
			return nil, err
		}
		r.Whole = s == token
		out = append(out, r)
		rest = rest[start+end+2:]
	}
}

func parseOneRef(token string) (ref, error) {
	body := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(token, "{{"), "}}"))
	switch {
	case strings.HasPrefix(body, "params."):
		name := strings.TrimPrefix(body, "params.")
		if !refName.MatchString(name) {
			return ref{}, fmt.Errorf("reference %q has a malformed name", token)
		}
		return ref{Kind: refParam, Name: name}, nil
	case strings.HasPrefix(body, "tasks."):
		parts := strings.Split(strings.TrimPrefix(body, "tasks."), ".")
		if len(parts) < 2 || parts[1] != "output" {
			return ref{}, fmt.Errorf("reference %q must name \"tasks.%s.output\"", token, parts[0])
		}
		if !refName.MatchString(parts[0]) {
			return ref{}, fmt.Errorf("reference %q has a malformed name", token)
		}
		var path []string
		for _, p := range parts[2:] {
			if !refField.MatchString(p) {
				return ref{}, fmt.Errorf("reference %q has a malformed name", token)
			}
			path = append(path, p)
		}
		return ref{Kind: refTaskOutput, Name: parts[0], Path: path}, nil
	}
	return ref{}, fmt.Errorf("reference %q must start with \"params.\" or \"tasks.\"", token)
}
