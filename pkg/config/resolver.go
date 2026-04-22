package config

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"
)

// placeholderPattern matches ${kind:spec[|filter…]} placeholders.
var placeholderPattern = regexp.MustCompile(`\$\{(env|file|vault|gen|ref):([^}|]+)((?:\|[^}]+)?)\}`)

// ResolverOpts configures secret resolution.
type ResolverOpts struct {
	// EnvName seeds gen:password deterministic output.
	EnvName string
	// LookupEnv overrides os.LookupEnv (useful for tests).
	LookupEnv func(string) (string, bool)
	// ReadFile overrides os.ReadFile (useful for tests).
	ReadFile func(string) ([]byte, error)
}

// ResolveResult captures the outcome of a resolver walk.
type ResolveResult struct {
	// Unresolved contains placeholders whose values could not be materialized
	// (e.g. vault: paths, gen:ssh_key — deferred to runtime).
	Unresolved []string
	// Errors accumulates hard failures (env var missing with no default, file read failure).
	Errors []error
}

// ResolvePlaceholders walks every string field in v and expands ${…} tokens.
// It mutates v in place.
func ResolvePlaceholders(v any, opts ResolverOpts) *ResolveResult {
	if opts.LookupEnv == nil {
		opts.LookupEnv = os.LookupEnv
	}
	if opts.ReadFile == nil {
		opts.ReadFile = os.ReadFile
	}
	res := &ResolveResult{}
	walkStrings(reflect.ValueOf(v), func(s string) string {
		return expandPlaceholders(s, opts, res, v)
	})
	return res
}

// expandPlaceholders replaces every ${…} token in s.
func expandPlaceholders(s string, opts ResolverOpts, res *ResolveResult, root any) string {
	return placeholderPattern.ReplaceAllStringFunc(s, func(raw string) string {
		m := placeholderPattern.FindStringSubmatch(raw)
		if m == nil {
			return raw
		}
		kind := m[1]
		spec := m[2]
		pipe := strings.TrimPrefix(m[3], "|")
		filters := []string{}
		if pipe != "" {
			for _, f := range strings.Split(pipe, "|") {
				filters = append(filters, strings.TrimSpace(f))
			}
		}

		var (
			value    string
			resolved = true
		)

		switch kind {
		case "env":
			val, ok := opts.LookupEnv(spec)
			if !ok || val == "" {
				def, hasDef := defaultFilter(filters)
				if hasDef {
					value = def
				} else {
					res.Errors = append(res.Errors, fmt.Errorf("env var %q not set and no default", spec))
					return raw
				}
			} else {
				value = val
			}
		case "file":
			path := expandHome(spec)
			b, err := opts.ReadFile(path)
			if err != nil {
				def, hasDef := defaultFilter(filters)
				if hasDef {
					value = def
				} else {
					res.Errors = append(res.Errors, fmt.Errorf("file %q: %w", spec, err))
					return raw
				}
			} else {
				value = strings.TrimRight(string(b), "\n")
			}
		case "vault":
			// Deferred: we don't dial Vault here.
			value = fmt.Sprintf("<VAULT:%s>", spec)
			resolved = false
			res.Unresolved = append(res.Unresolved, raw)
		case "gen":
			value = expandGen(spec, opts.EnvName)
			if strings.HasPrefix(spec, "ssh_key:") {
				resolved = false
				res.Unresolved = append(res.Unresolved, raw)
			}
		case "ref":
			v, err := resolveRefPath(root, spec)
			if err != nil {
				res.Errors = append(res.Errors, err)
				return raw
			}
			value = v
		default:
			return raw
		}

		// Apply non-default filters.
		for _, f := range filters {
			if strings.HasPrefix(f, "default:") {
				continue
			}
			switch f {
			case "base64":
				value = base64.StdEncoding.EncodeToString([]byte(value))
			}
		}

		_ = resolved
		return value
	})
}

// defaultFilter returns the value of a "|default:X" filter if present.
func defaultFilter(filters []string) (string, bool) {
	for _, f := range filters {
		if strings.HasPrefix(f, "default:") {
			return strings.TrimPrefix(f, "default:"), true
		}
	}
	return "", false
}

// expandGen handles gen:password:N[:SEED] and gen:ssh_key:TYPE.
func expandGen(spec, envName string) string {
	parts := strings.Split(spec, ":")
	if len(parts) == 0 {
		return ""
	}
	switch parts[0] {
	case "password":
		if len(parts) < 2 {
			return ""
		}
		n, err := strconv.Atoi(parts[1])
		if err != nil || n <= 0 {
			return ""
		}
		seed := ""
		if len(parts) >= 3 {
			seed = parts[2]
		}
		sum := sha256.Sum256([]byte(envName + ":" + seed))
		h := hex.EncodeToString(sum[:])
		if n > len(h) {
			n = len(h)
		}
		return h[:n]
	case "ssh_key":
		kind := "ed25519"
		if len(parts) >= 2 {
			kind = parts[1]
		}
		return fmt.Sprintf("<GEN_SSH_KEY:%s>", kind)
	}
	return ""
}

// resolveRefPath walks dotted path through the root env struct via reflection.
// Supports field names and map keys.
func resolveRefPath(root any, path string) (string, error) {
	v := reflect.ValueOf(root)
	parts := strings.Split(path, ".")
	for _, p := range parts {
		for v.Kind() == reflect.Ptr || v.Kind() == reflect.Interface {
			if v.IsNil() {
				return "", fmt.Errorf("ref %q: nil along path", path)
			}
			v = v.Elem()
		}
		switch v.Kind() {
		case reflect.Struct:
			next := v.FieldByNameFunc(func(name string) bool {
				return strings.EqualFold(name, p)
			})
			if !next.IsValid() {
				return "", fmt.Errorf("ref %q: no field %q", path, p)
			}
			v = next
		case reflect.Map:
			mv := v.MapIndex(reflect.ValueOf(p))
			if !mv.IsValid() {
				return "", fmt.Errorf("ref %q: no map key %q", path, p)
			}
			v = mv
		default:
			return "", fmt.Errorf("ref %q: cannot traverse %s", path, v.Kind())
		}
	}
	for v.Kind() == reflect.Ptr || v.Kind() == reflect.Interface {
		if v.IsNil() {
			return "", fmt.Errorf("ref %q: nil value", path)
		}
		v = v.Elem()
	}
	if !v.IsValid() {
		return "", fmt.Errorf("ref %q: invalid value", path)
	}
	return fmt.Sprintf("%v", v.Interface()), nil
}

// expandHome replaces leading ~/ with $HOME.
func expandHome(p string) string {
	if strings.HasPrefix(p, "~/") {
		if h, err := os.UserHomeDir(); err == nil {
			return filepath.Join(h, p[2:])
		}
	}
	return p
}

// walkStrings traverses v and calls fn on every settable string field,
// substituting the return value back in place.
func walkStrings(v reflect.Value, fn func(string) string) {
	if !v.IsValid() {
		return
	}
	switch v.Kind() {
	case reflect.Ptr, reflect.Interface:
		if !v.IsNil() {
			walkStrings(v.Elem(), fn)
		}
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			walkStrings(v.Field(i), fn)
		}
	case reflect.Slice, reflect.Array:
		for i := 0; i < v.Len(); i++ {
			walkStrings(v.Index(i), fn)
		}
	case reflect.Map:
		// We cannot set map values in place if they're non-addressable;
		// rebuild and reassign.
		iter := v.MapRange()
		updates := map[any]reflect.Value{}
		for iter.Next() {
			k := iter.Key()
			mv := iter.Value()
			newV, changed := transformMapValue(mv, fn)
			if changed {
				updates[k.Interface()] = newV
			}
		}
		for k, nv := range updates {
			v.SetMapIndex(reflect.ValueOf(k), nv)
		}
	case reflect.String:
		if v.CanSet() {
			old := v.String()
			newS := fn(old)
			if newS != old {
				v.SetString(newS)
			}
		}
	}
}

// transformMapValue returns a possibly-updated value for a map entry.
func transformMapValue(mv reflect.Value, fn func(string) string) (reflect.Value, bool) {
	// Must make a mutable copy.
	switch mv.Kind() {
	case reflect.String:
		old := mv.String()
		newS := fn(old)
		if newS != old {
			return reflect.ValueOf(newS).Convert(mv.Type()), true
		}
		return mv, false
	case reflect.Interface:
		if mv.IsNil() {
			return mv, false
		}
		inner := mv.Elem()
		if inner.Kind() == reflect.String {
			old := inner.String()
			newS := fn(old)
			if newS != old {
				return reflect.ValueOf(newS), true
			}
			return mv, false
		}
		// Recurse into the element via a temporary addressable copy.
		copy := reflect.New(inner.Type()).Elem()
		copy.Set(inner)
		walkStrings(copy, fn)
		if !reflect.DeepEqual(copy.Interface(), inner.Interface()) {
			return copy, true
		}
		return mv, false
	default:
		// Addressable copy.
		copy := reflect.New(mv.Type()).Elem()
		copy.Set(mv)
		walkStrings(copy, fn)
		if !reflect.DeepEqual(copy.Interface(), mv.Interface()) {
			return copy, true
		}
		return mv, false
	}
}

// WalkStringsForTest applies fn to every string in v (exported for callers
// that need to post-process loaded Env trees, e.g. redaction).
func WalkStringsForTest(v any, fn func(string) string) {
	walkStrings(reflect.ValueOf(v), fn)
}

// ParsePlaceholders is a lightweight syntax scan returning matched placeholders.
type Placeholder struct {
	Kind   string
	Spec   string
	Filter string
	Raw    string
}

// ParsePlaceholders returns every ${...} token found in input.
func ParsePlaceholders(input string) []Placeholder {
	matches := placeholderPattern.FindAllStringSubmatch(input, -1)
	out := make([]Placeholder, 0, len(matches))
	for _, m := range matches {
		out = append(out, Placeholder{Kind: m[1], Spec: m[2], Filter: strings.TrimPrefix(m[3], "|"), Raw: m[0]})
	}
	return out
}
