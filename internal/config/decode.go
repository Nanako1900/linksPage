package config

import (
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"

	"github.com/go-viper/mapstructure/v2"
)

// csvHook splits a string into a []string on commas, trimming blanks, so
// that list keys can be set from a single environment variable.
func csvHook(from, to reflect.Type, data any) (any, error) {
	if from.Kind() != reflect.String || to != reflect.TypeFor[[]string]() {
		return data, nil
	}
	parts := strings.Split(data.(string), ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out, nil
}

// decode decodes raw into out. It returns the unused (unknown) keys and a
// sanitized error that only names key paths, never values.
func decode(raw any, out any, prefix string) ([]string, error) {
	var md mapstructure.Metadata
	dec, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		TagName:          "koanf",
		WeaklyTypedInput: true,
		Metadata:         &md,
		Result:           out,
		DecodeHook: mapstructure.ComposeDecodeHookFunc(
			mapstructure.StringToTimeDurationHookFunc(),
			csvHook,
		),
	})
	if err != nil {
		return nil, fmt.Errorf("config decoder: %w", err)
	}
	if err := dec.Decode(raw); err != nil {
		return nil, sanitizeDecodeError(err, prefix)
	}
	unused := make([]string, 0, len(md.Unused))
	for _, k := range md.Unused {
		unused = append(unused, prefix+k)
	}
	slices.Sort(unused)
	return unused, nil
}

// sanitizeDecodeError rewrites mapstructure errors so that only key paths
// are reported. mapstructure messages may embed the offending value.
func sanitizeDecodeError(err error, prefix string) error {
	paths := map[string]bool{}
	collectDecodePaths(err, paths)
	if len(paths) == 0 {
		return fmt.Errorf("%sinvalid value type", strings.TrimSuffix(prefix, ".")+": ")
	}
	sorted := make([]string, 0, len(paths))
	for p := range paths {
		sorted = append(sorted, p)
	}
	slices.Sort(sorted)
	errs := make([]error, 0, len(sorted))
	for _, p := range sorted {
		errs = append(errs, fmt.Errorf("%s%s: invalid value type", prefix, p))
	}
	return errors.Join(errs...)
}

func collectDecodePaths(err error, paths map[string]bool) {
	if err == nil {
		return
	}
	var de *mapstructure.DecodeError
	if errors.As(err, &de) {
		// Use the deepest DecodeError name; nested errors carry full paths.
		inner := errors.Unwrap(de)
		before := len(paths)
		collectDecodePaths(inner, paths)
		if len(paths) == before && de.Name() != "" {
			paths[de.Name()] = true
		}
		return
	}
	if multi, ok := err.(interface{ Unwrap() []error }); ok {
		for _, e := range multi.Unwrap() {
			collectDecodePaths(e, paths)
		}
	}
}
