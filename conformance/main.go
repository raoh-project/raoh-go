// Command conformance runs the cases of the Raoh Specification on raoh-go and
// writes a runner result (spec/conformance.md), which raoh-verify checks
// against conformance/conformance.json. scripts/conformance.sh runs both.
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/raoh-project/raoh-go"
	"github.com/raoh-project/raoh-go/encode"
)

func main() {
	spec := flag.String("spec", "", "a checkout of raoh-specification")
	revision := flag.String("revision", "", "the commit of raoh-specification the cases are read at")
	digest := flag.String("manifest-digest", "", "the manifest digest raoh-verify gives for that commit")
	implRevision := flag.String("implementation-revision", "", "the commit of raoh-go")
	implVersion := flag.String("implementation-version", "devel", "the version of raoh-go, X.Y.Z when the commit is a release")
	messages := flag.String("messages", "../messages", "the directory of raoh-go's message catalogues")
	out := flag.String("out", "runner-result.json", "where to write the runner result")
	flag.Parse()
	if err := run(*spec, *revision, *digest, *implRevision, *implVersion, *messages, *out); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(spec, revision, digest, implRevision, implVersion, messages, out string) error {
	version, err := specificationVersion(spec)
	if err != nil {
		return err
	}
	bound, err := readCatalogue(spec)
	if err != nil {
		return err
	}
	results := map[string]any{}
	for _, profile := range []string{"core", "encode"} {
		files, _ := filepath.Glob(filepath.Join(spec, "suite", profile, "*.json"))
		sort.Strings(files)
		for _, file := range files {
			cases, err := readCases(file)
			if err != nil {
				return err
			}
			for _, c := range cases {
				observed, ran := runCase(c, bound)
				if ran {
					results[c.ID] = map[string]any{"observed": observed}
				}
			}
		}
	}
	catalogs, err := readCatalogs(messages)
	if err != nil {
		return err
	}
	features := make([]string, 0, len(bound))
	for f := range bound {
		features = append(features, f)
	}
	sort.Strings(features)
	result := map[string]any{
		"format":        "raoh-runner-result/v1",
		"specification": map[string]any{"version": version, "revision": revision, "manifest_digest": digest},
		"implementation": map[string]any{
			"name": "raoh-go", "version": implVersion, "revision": implRevision,
		},
		"environment": map[string]any{
			"language": "go", "language_version": runtime.Version(), "os": runtime.GOOS, "arch": runtime.GOARCH,
		},
		"bound_features": features,
		"results":        results,
		"catalogs":       catalogs,
	}
	text, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(out, append(text, '\n'), 0o644)
}

// aCase is a case of the suite, its forms and observations read with every
// number kept as written.
type aCase struct {
	ID      string          `json:"id"`
	Decoder any             `json:"decoder"`
	Encoder any             `json:"encoder"`
	Input   json.RawMessage `json:"input"`
	Value   any             `json:"value"`
}

func readCases(file string) ([]aCase, error) {
	text, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	var raw []json.RawMessage
	if err := json.Unmarshal(text, &raw); err != nil {
		return nil, fmt.Errorf("%s: %w", file, err)
	}
	cases := make([]aCase, len(raw))
	for n, r := range raw {
		d := json.NewDecoder(bytes.NewReader(r))
		d.UseNumber()
		if err := d.Decode(&cases[n]); err != nil {
			return nil, fmt.Errorf("%s: %w", file, err)
		}
	}
	return cases, nil
}

// runCase runs a case whose features are all bound, and answers what it
// observed; ran is false where the case needs a feature the runner does not
// bind.
func runCase(c aCase, bound map[string]bool) (observed any, ran bool) {
	b := &binder{features: map[string]bool{}}
	defer func() {
		if r := recover(); r != nil {
			observed, ran = map[string]any{"error": fmt.Sprint(r)}, true
		}
	}()
	if c.Encoder != nil {
		ok, err := b.encode(c.Encoder, c.Value)
		if !allBound(b.features, bound) {
			return nil, false
		}
		if err != nil {
			return map[string]any{"error": err.Error()}, true
		}
		return map[string]any{"ok": ok}, true
	}
	n, err := b.decoder(c.Decoder)
	if !allBound(b.features, bound) {
		// A case that needs a feature the runner does not bind is not run,
		// whether or not binding it went further.
		return nil, false
	}
	if err != nil {
		return map[string]any{"error": err.Error()}, true
	}
	v, err := raoh.DecodeJSON([]byte(c.Input), n.g)
	if err != nil {
		var issues *raoh.Issues
		if !errors.As(err, &issues) {
			return map[string]any{"error": err.Error()}, true
		}
		written, err := writeIssues(*issues)
		if err != nil {
			return map[string]any{"error": err.Error()}, true
		}
		return map[string]any{"issues": written}, true
	}
	o, err := observe(n.t, v)
	if err != nil {
		return map[string]any{"error": err.Error()}, true
	}
	return map[string]any{"ok": o}, true
}

func allBound(used, bound map[string]bool) bool {
	for f := range used {
		if !bound[f] {
			return false
		}
	}
	return true
}

// writeIssues writes issues as a case writes them, with the English
// messages.
func writeIssues(issues raoh.Issues) ([]any, error) {
	all := issues.All()
	rendered := issues.Render(raoh.English)
	out := make([]any, len(all))
	for n, i := range all {
		meta, err := observeMeta(rendered[n].Meta)
		if err != nil {
			return nil, err
		}
		out[n] = map[string]any{
			"path": rendered[n].Path, "code": i.Code(), "message_key": i.MessageKey(),
			"message": rendered[n].Message, "meta": meta,
		}
	}
	return out, nil
}

// encode runs an encoding case.
func (b *binder) encode(form, value any) (any, error) {
	f := form.([]any)
	b.use("encoder." + f[0].(string))
	switch f[0] {
	case "string":
		v, err := materialise(tString, value)
		if err != nil {
			return nil, err
		}
		return encode.String()(v.(string)), nil
	case "object":
		properties := f[1].([]any)
		if len(properties) != 1 {
			return nil, fmt.Errorf("no binding of an object encoder of %d properties", len(properties))
		}
		p := properties[0].([]any)
		b.use("property." + p[0].(string))
		if p[0] != "propertyWithDefault" || p[2] != "identity" {
			return nil, fmt.Errorf("no binding of the property %v", p)
		}
		b.use("fixture.identity")
		enc := p[3].([]any)
		b.use("encoder." + enc[0].(string))
		if enc[0] != "string" {
			return nil, fmt.Errorf("no binding of the encoder %v in a property", enc)
		}
		v, err := materialise(&typ{kind: "nullable", args: []*typ{tString}}, value)
		if err != nil {
			return nil, err
		}
		var given *string
		if p := v.(*any); p != nil {
			s := (*p).(string)
			given = &s
		}
		identity := func(s *string) *string { return s }
		o := encode.Object(encode.PropertyWithDefault(p[1].(string), identity, encode.String(), p[4].(string)))
		return o(given), nil
	}
	return nil, fmt.Errorf("no encoder %v", f[0])
}

func specificationVersion(spec string) (string, error) {
	text, err := os.ReadFile(filepath.Join(spec, "specification.json"))
	if err != nil {
		return "", err
	}
	var v struct {
		Version string `json:"version"`
	}
	return v.Version, json.Unmarshal(text, &v)
}

// readCatalogue reads catalog/operations.json and catalog/fixtures.json for
// the features the runner binds, and the shapes of the operations' arguments.
func readCatalogue(spec string) (map[string]bool, error) {
	var ops struct {
		Constructors map[string]struct {
			Args []struct{ Kind string } `json:"args"`
		} `json:"constructors"`
		Fields     map[string]any `json:"fields"`
		Operations []struct {
			Name      string                  `json:"name"`
			Receivers []string                `json:"receivers"`
			Args      []struct{ Kind string } `json:"args"`
		} `json:"operations"`
		Encoders   map[string]any `json:"encoders"`
		Properties map[string]any `json:"properties"`
	}
	text, err := os.ReadFile(filepath.Join(spec, "catalog", "operations.json"))
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(text, &ops); err != nil {
		return nil, err
	}
	// What the catalogue has and the runner binds: a feature added to the
	// catalogue is bound once the runner has a binding of it, and not before.
	bound := map[string]bool{}
	bind := func(feature string) {
		if binds[feature] {
			bound[feature] = true
		}
	}
	for name, c := range ops.Constructors {
		bind("decoder." + name)
		for _, a := range c.Args {
			if a.Kind == "message" {
				bind("decoder." + name + ".message")
			}
		}
	}
	for name := range ops.Fields {
		bind("field." + name)
	}
	for _, o := range ops.Operations {
		shape := operationShape{}
		for _, a := range o.Args {
			if a.Kind == "message" {
				shape.message = true
			} else {
				shape.values++
			}
		}
		operationShapes[o.Name] = shape
		for _, r := range o.Receivers {
			kind, _, _ := strings.Cut(r, "<")
			if kind == "*" {
				kind = "any"
			}
			bind("operation." + kind + "." + o.Name)
			if shape.message {
				bind("operation." + kind + "." + o.Name + ".message")
			}
		}
	}
	for name := range ops.Encoders {
		bind("encoder." + name)
	}
	for name := range ops.Properties {
		bind("property." + name)
	}
	text, err = os.ReadFile(filepath.Join(spec, "catalog", "fixtures.json"))
	if err != nil {
		return nil, err
	}
	var fixtures map[string]any
	if err := json.Unmarshal(text, &fixtures); err != nil {
		return nil, err
	}
	for name := range fixtures {
		bind("fixture." + name)
	}
	return bound, nil
}

// readCatalogs reads the message catalogues raoh-go ships, every key without
// the prefix raoh.
func readCatalogs(dir string) (map[string]any, error) {
	out := map[string]any{}
	for _, locale := range []string{"en", "ja"} {
		text, err := os.ReadFile(filepath.Join(dir, locale+".properties"))
		if err != nil {
			return nil, err
		}
		m, err := raoh.ParseProperties(string(text))
		if err != nil {
			return nil, err
		}
		templates := map[string]any{}
		for k, v := range m.Templates() {
			templates[strings.TrimPrefix(k, "raoh.")] = v
		}
		out[locale] = templates
	}
	return out, nil
}
