// Package typescript generates a fetch client and its types from the IR.
package typescript

import (
	"bytes"
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"text/template"
	"unicode"
	"unicode/utf8"

	"github.com/atharv1730/SpecForge/internal/gen"
	"github.com/atharv1730/SpecForge/internal/ir"
)

//go:embed templates/*.tmpl
var templateFiles embed.FS

// Generator writes types.ts and client.ts.
type Generator struct{}

func init() {
	gen.Register("typescript", Generator{})
}

// Generate renders the TypeScript client for api into outputDir.
func (Generator) Generate(api *ir.API, outputDir string) error {
	if api == nil {
		return fmt.Errorf("typescript: nil api")
	}
	if outputDir == "" {
		return fmt.Errorf("typescript: empty output directory")
	}
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return err
	}
	data := prepare(api)
	for _, name := range []string{"types.ts", "client.ts"} {
		var buf bytes.Buffer
		if err := files.ExecuteTemplate(&buf, name+".tmpl", data); err != nil {
			return fmt.Errorf("render %s: %w", name, err)
		}
		body := tidy(buf.Bytes())
		if err := os.WriteFile(filepath.Join(outputDir, name), body, 0o644); err != nil {
			return err
		}
	}
	return nil
}

var (
	ident      = regexp.MustCompile(`^[A-Za-z_$][A-Za-z0-9_$]*$`)
	extraBlank = regexp.MustCompile(`\n{3,}`)
)

var funcs = template.FuncMap{
	"camelCase": camelCase,
	"tsType":    tsType,
	"fieldName": fieldName,
	"arrayType": arrayType,
	"enumType":  enumType,
	"quote":     strconv.Quote,
	"comment":   comment,
}

var files = template.Must(template.New("ts").Funcs(funcs).ParseFS(templateFiles, "templates/*.tmpl"))

type view struct {
	BaseURL string
	Imports []string
	Types   []typeDecl
	Methods []methodView
}

type typeDecl struct {
	Name string
	Kind string
	Type ir.TypeRef
}

type methodView struct {
	OperationID string
	Summary     string
	Method      string
	PathExpr    string
	Args        []argView
	Queries     []argView
	Headers     []argView
	Body        *argView
	ContentType string
	HasReturn   bool
	Return      ir.TypeRef
}

type argView struct {
	Name     string
	Wire     string
	Required bool
	Schema   ir.TypeRef
}

func prepare(api *ir.API) view {
	seen := map[string]bool{}
	named := map[string]ir.TypeRef{}
	methods := make([]methodView, 0, len(api.Endpoints))
	used := map[string]bool{}
	for _, ep := range api.Endpoints {
		method := methodView{
			OperationID: ep.OperationID,
			Summary:     ep.Summary,
			Method:      ep.Method,
			ContentType: "application/json",
		}
		var args []argView
		taken := map[string]bool{}
		for _, p := range ep.Parameters {
			collect(named, seen, p.Schema)
			arg := argView{
				Name:     uniqueName(camelCase(p.Name), taken),
				Wire:     p.Name,
				Required: p.Required || p.In == "path",
				Schema:   p.Schema,
			}
			switch p.In {
			case "query":
				method.Queries = append(method.Queries, arg)
			case "header":
				method.Headers = append(method.Headers, arg)
			}
			args = append(args, arg)
		}
		if ep.RequestBody != nil {
			collect(named, seen, ep.RequestBody.Schema)
			if ep.RequestBody.ContentType != "" {
				method.ContentType = ep.RequestBody.ContentType
			}
			body := argView{
				Name:     uniqueName("body", taken),
				Required: ep.RequestBody.Required,
				Schema:   ep.RequestBody.Schema,
			}
			method.Body = &body
			args = append(args, body)
		}
		method.Args = requiredFirst(args)
		method.PathExpr = pathExpr(ep.Path, wireNames(args))
		if ret, ok := successType(ep); ok {
			collect(named, seen, ret)
			method.HasReturn = true
			method.Return = ret
			noteRefs(ret, used)
		}
		for _, arg := range method.Args {
			noteRefs(arg.Schema, used)
		}
		for _, resp := range ep.Responses {
			if resp.Schema != nil {
				collect(named, seen, *resp.Schema)
			}
		}
		methods = append(methods, method)
	}

	names := make([]string, 0, len(named))
	for name := range named {
		names = append(names, name)
	}
	sort.Strings(names)
	types := make([]typeDecl, 0, len(names))
	for _, name := range names {
		t := named[name]
		types = append(types, typeDecl{Name: name, Kind: t.Kind, Type: t})
	}
	imports := make([]string, 0, len(used))
	for name := range used {
		if _, ok := named[name]; ok {
			imports = append(imports, name)
		}
	}
	sort.Strings(imports)
	return view{BaseURL: api.BaseURL, Imports: imports, Types: types, Methods: methods}
}

func wireNames(args []argView) map[string]string {
	names := make(map[string]string, len(args))
	for _, arg := range args {
		if arg.Wire != "" {
			names[arg.Wire] = arg.Name
		}
	}
	return names
}

func requiredFirst(args []argView) []argView {
	out := make([]argView, 0, len(args))
	for _, arg := range args {
		if arg.Required {
			out = append(out, arg)
		}
	}
	for _, arg := range args {
		if !arg.Required {
			out = append(out, arg)
		}
	}
	return out
}

func successType(ep ir.Endpoint) (ir.TypeRef, bool) {
	var best *ir.Response
	for i := range ep.Responses {
		resp := &ep.Responses[i]
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			continue
		}
		if best == nil || resp.StatusCode < best.StatusCode {
			best = resp
		}
	}
	if best == nil || best.Schema == nil {
		return ir.TypeRef{}, false
	}
	return *best.Schema, true
}

func collect(dst map[string]ir.TypeRef, seen map[string]bool, t ir.TypeRef) {
	if isNamed(t) {
		if seen[t.Name] {
			return
		}
		seen[t.Name] = true
		dst[t.Name] = t
	}
	switch t.Kind {
	case "object":
		for _, prop := range t.Properties {
			collect(dst, seen, prop.Type)
		}
	case "array":
		if t.Items != nil {
			collect(dst, seen, *t.Items)
		}
	}
}

func noteRefs(t ir.TypeRef, dst map[string]bool) {
	if isNamed(t) {
		dst[t.Name] = true
		return
	}
	switch t.Kind {
	case "object":
		for _, prop := range t.Properties {
			noteRefs(prop.Type, dst)
		}
	case "array":
		if t.Items != nil {
			noteRefs(*t.Items, dst)
		}
	}
}

func isNamed(t ir.TypeRef) bool {
	if t.Name == "" {
		return false
	}
	switch t.Kind {
	case "object", "array":
		return true
	case "enum":
		return !isPrimitive(t.Name)
	default:
		return false
	}
}

func isPrimitive(name string) bool {
	switch name {
	case "string", "integer", "number", "boolean":
		return true
	default:
		return false
	}
}

func tsType(t ir.TypeRef) string {
	if isNamed(t) {
		return t.Name
	}
	switch t.Kind {
	case "enum":
		return enumType(t)
	case "array":
		return arrayType(t)
	case "object":
		return inlineObject(t)
	default:
		switch t.Name {
		case "integer", "number":
			return "number"
		case "boolean":
			return "boolean"
		default:
			return "string"
		}
	}
}

func arrayType(t ir.TypeRef) string {
	if t.Items == nil {
		return "unknown[]"
	}
	inner := tsType(*t.Items)
	if strings.Contains(inner, "|") || strings.HasPrefix(inner, "{") {
		inner = "(" + inner + ")"
	}
	return inner + "[]"
}

func enumType(t ir.TypeRef) string {
	if len(t.Enum) == 0 {
		return "never"
	}
	numeric := t.Name == "integer" || t.Name == "number"
	parts := make([]string, len(t.Enum))
	for i, value := range t.Enum {
		if numeric && numericLiteral(value) {
			parts[i] = value
			continue
		}
		parts[i] = strconv.Quote(value)
	}
	return strings.Join(parts, " | ")
}

func numericLiteral(value string) bool {
	if value == "" || value == "-" || value == "." {
		return false
	}
	if _, err := strconv.ParseFloat(value, 64); err != nil {
		return false
	}
	for _, r := range value {
		if (r < '0' || r > '9') && r != '-' && r != '.' {
			return false
		}
	}
	return true
}

func inlineObject(t ir.TypeRef) string {
	if len(t.Properties) == 0 {
		return "Record<string, never>"
	}
	var b strings.Builder
	b.WriteString("{ ")
	for i, prop := range t.Properties {
		if i > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(fieldName(prop.Name))
		if !prop.Required {
			b.WriteByte('?')
		}
		b.WriteString(": ")
		b.WriteString(tsType(prop.Type))
		b.WriteByte(';')
	}
	b.WriteString(" }")
	return b.String()
}

func fieldName(name string) string {
	if ident.MatchString(name) {
		return name
	}
	return strconv.Quote(name)
}

func camelCase(s string) string {
	parts := strings.FieldsFunc(s, func(r rune) bool {
		return r == '_' || r == '-' || r == ' '
	})
	var b strings.Builder
	for i, part := range parts {
		if part == "" {
			continue
		}
		if i == 0 {
			b.WriteString(lowerFirst(part))
			continue
		}
		b.WriteString(upperFirst(part))
	}
	out := b.String()
	if out == "" {
		return "arg"
	}
	if !ident.MatchString(out) {
		out = "_" + out
	}
	return out
}

func lowerFirst(s string) string {
	r, size := utf8.DecodeRuneInString(s)
	return string(unicode.ToLower(r)) + s[size:]
}

func upperFirst(s string) string {
	r, size := utf8.DecodeRuneInString(s)
	return string(unicode.ToUpper(r)) + s[size:]
}

func uniqueName(name string, taken map[string]bool) string {
	if name == "" {
		name = "arg"
	}
	if !taken[name] {
		taken[name] = true
		return name
	}
	for i := 2; ; i++ {
		next := fmt.Sprintf("%s%d", name, i)
		if !taken[next] {
			taken[next] = true
			return next
		}
	}
}

func pathExpr(path string, names map[string]string) string {
	var b strings.Builder
	for i := 0; i < len(path); {
		if path[i] == '{' {
			if j := strings.IndexByte(path[i:], '}'); j > 1 {
				raw := path[i+1 : i+j]
				name := names[raw]
				if name == "" {
					name = camelCase(raw)
				}
				b.WriteString("${encodeURIComponent(")
				b.WriteString(name)
				b.WriteString(")}")
				i += j + 1
				continue
			}
		}
		switch path[i] {
		case '\\', '`':
			b.WriteByte('\\')
			b.WriteByte(path[i])
		case '$':
			b.WriteString("\\$")
		default:
			b.WriteByte(path[i])
		}
		i++
	}
	return b.String()
}

func tidy(b []byte) []byte {
	b = bytes.TrimSpace(b)
	b = extraBlank.ReplaceAll(b, []byte("\n\n"))
	return append(b, '\n')
}

func comment(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	return strings.ReplaceAll(s, "*/", "* /")
}
