// Package python generates a urllib client and its models from the IR.
package python

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

// Generator writes models.py and client.py.
type Generator struct{}

func init() {
	gen.Register("python", Generator{})
}

// Generate renders the Python client for api into outputDir.
func (Generator) Generate(api *ir.API, outputDir string) error {
	if api == nil {
		return fmt.Errorf("cannot generate a Python client: API is nil. Load an OpenAPI spec before generating")
	}
	if outputDir == "" {
		return fmt.Errorf("cannot generate a Python client: output directory is empty. Pass a directory to --output")
	}
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return fmt.Errorf("cannot create output directory %q: %w. Check that the parent directory exists and is writable", outputDir, err)
	}
	data := prepare(api)
	for _, name := range []string{"models.py", "client.py"} {
		var buf bytes.Buffer
		if err := files.ExecuteTemplate(&buf, name+".tmpl", data); err != nil {
			return fmt.Errorf("cannot render %s: %w. The Python generator failed while writing the client", name, err)
		}
		body := tidy(buf.Bytes())
		path := filepath.Join(outputDir, name)
		if err := os.WriteFile(path, body, 0o644); err != nil {
			return fmt.Errorf("cannot write %s: %w. Check that the output directory is writable", path, err)
		}
	}
	return nil
}

var extraBlank = regexp.MustCompile(`\n{3,}`)

var keywords = map[string]bool{
	"False": true, "None": true, "True": true,
	"and": true, "as": true, "assert": true, "async": true, "await": true,
	"break": true, "class": true, "continue": true, "def": true, "del": true,
	"elif": true, "else": true, "except": true, "finally": true, "for": true,
	"from": true, "global": true, "if": true, "import": true, "in": true,
	"is": true, "lambda": true, "nonlocal": true, "not": true, "or": true,
	"pass": true, "raise": true, "return": true, "try": true, "while": true,
	"with": true, "yield": true,
}

// reservedType names would shadow builtins used in the generated annotations.
var reservedType = map[string]bool{
	"str": true, "int": true, "float": true, "bool": true, "list": true,
	"dict": true, "set": true, "tuple": true, "object": true, "bytes": true,
}

var funcs = template.FuncMap{
	"pyType":    pyType,
	"fieldName": fieldName,
	"arrayType": arrayType,
	"enumType":  enumType,
	"props":     props,
	"quote":     strconv.Quote,
	"comment":   comment,
}

var files = template.Must(template.New("py").Funcs(funcs).ParseFS(templateFiles, "templates/*.tmpl"))

type view struct {
	BaseURL       string
	ModelPrelude  string
	ClientPrelude string
	NeedParam     bool
	NeedBody      bool
	Types         []typeDecl
	Methods       []methodView
}

type typeDecl struct {
	Name string
	Kind string
	Type ir.TypeRef
}

type methodView struct {
	Name        string
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
	methodTaken := map[string]bool{"_param": true, "_body": true}
	for _, ep := range api.Endpoints {
		method := methodView{
			Name:        uniqueName(snakeCase(ep.OperationID), methodTaken),
			Summary:     ep.Summary,
			Method:      ep.Method,
			ContentType: "application/json",
		}
		var args []argView
		taken := map[string]bool{
			"self": true, "root": true, "params": true, "qs": true,
			"url": true, "headers": true, "data": true, "req": true,
			"res": true, "raw": true, "err": true, "_param": true, "_body": true,
		}
		for _, p := range ep.Parameters {
			collect(named, seen, p.Schema)
			arg := argView{
				Name:     uniqueName(snakeCase(p.Name), taken),
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
		types = append(types, typeDecl{Name: typeName(name), Kind: t.Kind, Type: t})
	}
	v := view{BaseURL: api.BaseURL, Types: types, Methods: methods}
	v.ModelPrelude = modelPrelude(types)
	v.ClientPrelude, v.NeedParam, v.NeedBody = clientPrelude(v)
	return v
}

func modelPrelude(types []typeDecl) string {
	needClass := false
	needLiteral := false
	needNever := false
	for _, decl := range types {
		if decl.Kind == "object" {
			needClass = true
		}
		lit, never := typingNeeds(decl.Type, true)
		needLiteral = needLiteral || lit
		needNever = needNever || never
	}
	var lines []string
	if needClass {
		lines = append(lines, "from dataclasses import dataclass")
	}
	if line := typingImport(false, needLiteral, needNever); line != "" {
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func clientPrelude(v view) (string, bool, bool) {
	needJSON := false
	needCast := false
	needLiteral := false
	needNever := false
	needQuote := false
	needQuery := false
	needParam := false
	needBody := false
	refs := map[string]bool{}
	for _, m := range v.Methods {
		if strings.Contains(m.PathExpr, "_param(") {
			needParam = true
			needQuote = true
		}
		if len(m.Queries) > 0 {
			needQuery = true
			needParam = true
		}
		if len(m.Headers) > 0 {
			needParam = true
		}
		if m.Body != nil {
			needJSON = true
			needBody = true
		}
		if m.HasReturn {
			needJSON = true
			needCast = true
			lit, never := typingNeeds(m.Return, false)
			needLiteral = needLiteral || lit
			needNever = needNever || never
			noteRefs(m.Return, refs)
		}
		for _, arg := range m.Args {
			lit, never := typingNeeds(arg.Schema, false)
			needLiteral = needLiteral || lit
			needNever = needNever || never
			noteRefs(arg.Schema, refs)
		}
	}
	var std []string
	if needJSON {
		std = append(std, "import json")
	}
	if needBody {
		std = append(std, "from dataclasses import asdict, is_dataclass")
	}
	if line := typingImport(needCast, needLiteral, needNever); line != "" {
		std = append(std, line)
	}
	std = append(std, "from urllib.error import HTTPError")
	var parse []string
	if needQuote {
		parse = append(parse, "quote")
	}
	if needQuery {
		parse = append(parse, "urlencode")
	}
	if len(parse) > 0 {
		std = append(std, "from urllib.parse import "+strings.Join(parse, ", "))
	}
	std = append(std, "from urllib.request import Request, urlopen")

	var b strings.Builder
	b.WriteString(strings.Join(std, "\n"))
	if len(refs) > 0 {
		names := make([]string, 0, len(refs))
		seen := map[string]bool{}
		for name := range refs {
			n := typeName(name)
			if seen[n] {
				continue
			}
			seen[n] = true
			names = append(names, n)
		}
		sort.Strings(names)
		b.WriteString("\n\nfrom models import ")
		b.WriteString(strings.Join(names, ", "))
	}
	return b.String(), needParam, needBody
}

func typingImport(cast, literal, never bool) string {
	var names []string
	if cast {
		names = append(names, "cast")
	}
	if literal {
		names = append(names, "Literal")
	}
	if never {
		names = append(names, "Never")
	}
	if len(names) == 0 {
		return ""
	}
	return "from typing import " + strings.Join(names, ", ")
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

func typingNeeds(t ir.TypeRef, decl bool) (literal, never bool) {
	if !decl && isNamed(t) {
		return false, false
	}
	switch t.Kind {
	case "enum":
		if len(t.Enum) == 0 {
			return false, true
		}
		return true, false
	case "array":
		if t.Items != nil {
			return typingNeeds(*t.Items, false)
		}
	case "object":
		for _, prop := range t.Properties {
			lit, nev := typingNeeds(prop.Type, false)
			literal = literal || lit
			never = never || nev
		}
		return literal, never
	}
	return false, false
}

func pyType(t ir.TypeRef) string {
	if isNamed(t) {
		return typeName(t.Name)
	}
	switch t.Kind {
	case "enum":
		return enumType(t)
	case "array":
		return arrayType(t)
	case "object":
		return "dict[str, object]"
	default:
		switch t.Name {
		case "integer":
			return "int"
		case "number":
			return "float"
		case "boolean":
			return "bool"
		default:
			return "str"
		}
	}
}

func arrayType(t ir.TypeRef) string {
	if t.Items == nil {
		return "list[object]"
	}
	return "list[" + pyType(*t.Items) + "]"
}

func enumType(t ir.TypeRef) string {
	if len(t.Enum) == 0 {
		return "Never"
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
	return "Literal[" + strings.Join(parts, ", ") + "]"
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

func props(t ir.TypeRef) []ir.Property {
	req := make([]ir.Property, 0, len(t.Properties))
	opt := make([]ir.Property, 0, len(t.Properties))
	for _, prop := range t.Properties {
		if prop.Required {
			req = append(req, prop)
		} else {
			opt = append(opt, prop)
		}
	}
	return append(req, opt...)
}

func fieldName(name string) string {
	if isIdent(name) && !isKeyword(name) {
		return name
	}
	return snakeCase(name)
}

func typeName(name string) string {
	if isIdent(name) && !isKeyword(name) && !reservedType[name] {
		return name
	}
	var b strings.Builder
	for _, r := range name {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' {
			b.WriteRune(r)
			continue
		}
		b.WriteByte('_')
	}
	out := strings.Trim(b.String(), "_")
	first, _ := utf8.DecodeRuneInString(out)
	if out == "" || unicode.IsDigit(first) {
		out = "Model_" + out
	}
	if !isIdent(out) || isKeyword(out) || reservedType[out] {
		out += "_"
	}
	return out
}

func snakeCase(s string) string {
	if isSnake(s) && !isKeyword(s) {
		return s
	}
	runes := []rune(s)
	var b strings.Builder
	prevSep := true
	for i, r := range runes {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			if b.Len() > 0 && !prevSep {
				b.WriteByte('_')
				prevSep = true
			}
			continue
		}
		if unicode.IsUpper(r) {
			nextLower := i+1 < len(runes) && unicode.IsLower(runes[i+1])
			prevLower := i > 0 && (unicode.IsLower(runes[i-1]) || unicode.IsDigit(runes[i-1]))
			prevUpper := i > 0 && unicode.IsUpper(runes[i-1])
			if b.Len() > 0 && !prevSep && (prevLower || (prevUpper && nextLower)) {
				b.WriteByte('_')
			}
			b.WriteRune(unicode.ToLower(r))
			prevSep = false
			continue
		}
		b.WriteRune(r)
		prevSep = false
	}
	out := strings.Trim(b.String(), "_")
	if out == "" {
		out = "arg"
	}
	if !isIdent(out) {
		out = "_" + out
	}
	if isKeyword(out) {
		out += "_"
	}
	return out
}

func isSnake(s string) bool {
	if !isIdent(s) {
		return false
	}
	for _, r := range s {
		if unicode.IsUpper(r) {
			return false
		}
	}
	return true
}

func isIdent(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		if i == 0 {
			if r != '_' && !unicode.IsLetter(r) {
				return false
			}
			continue
		}
		if r != '_' && !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			return false
		}
	}
	return true
}

func isKeyword(name string) bool {
	return keywords[name]
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
					name = snakeCase(raw)
				}
				b.WriteString("{quote(_param(")
				b.WriteString(name)
				b.WriteString(`), safe="")}`)
				i += j + 1
				continue
			}
		}
		switch path[i] {
		case '{':
			b.WriteString("{{")
		case '}':
			b.WriteString("}}")
		case '\\', '\'':
			b.WriteByte('\\')
			b.WriteByte(path[i])
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
	return strings.ReplaceAll(s, `"""`, `" " "`)
}
