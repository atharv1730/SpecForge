# Design

## Why Cobra

SpecForge is three commands (`generate`, `mock`, and `conformance`) with different flag sets. `spf13/cobra` models that as a command tree: each command owns its flags, help text, and required-flag checks. `MarkFlagRequired` is how `--spec`, `--output`, and `--language` fail closed when a caller omits them, without a hand-written presence check per flag. `--verbose` is a persistent flag on the root, so every command inherits it.

`urfave/cli` can express the same tree and also has required flags. It is a smaller API. Cobra still wins for this tool because its help output, `--version` handling, and command objects are what we assert against in tests (`SetArgs`, `SetOut`, `Execute`). One well-known shape is cheaper than a second CLI style we would only use here.

The standard library `flag` package parses a single flag set. Subcommands mean a `FlagSet` per command, a manual dispatcher, hand-written help, and hand-written required-flag errors. That is less to import and more to keep correct. Conformance is the command that would have forced us to own that dispatcher. The dependency cost of Cobra is one module; the cost of `flag` is owning the CLI framework ourselves.

## Why kin-openapi

The loader runs once. It turns an OpenAPI 3.x document into a typed model, and everything after that walks our IR, not the spec. `github.com/getkin/kin-openapi` is the loader most Go OpenAPI tools already use for that load, resolve, and validate step, so its failure modes are familiar and its API is stable enough to hide behind the IR boundary.

`github.com/pb33f/libopenapi` covers OpenAPI 3.1 more completely and exposes a lower-level AST plus its own index and resolver. We would not keep that AST. The next step collapses the document into the IR, so the extra model is a second thing to learn for a single parse.

The tradeoff we accept: kin-openapi has lagged libopenapi on some 3.1 features (webhooks, JSON Schema constructs that exist only in 3.1, certain keywords beside `$ref`). If a spec we must support hits that gap, the swap stays inside the loader. Until then the smaller, more widely deployed library is the one we parse with.

## IR design philosophy

Parse once, generate many. The loader reads an OpenAPI 3.x document a single time and emits an `ir.API`. The TypeScript client, the Python client, and the mock server all walk that same value. A new language is another walk of the IR, not another parser and not another pass over the spec.

The IR is the contract between the loader and every consumer. Generators may depend on the field names, on `TypeRef.Kind` (`primitive`, `object`, `array`, `enum`), and on which pointers are nil (`Endpoint.RequestBody`, `Response.Schema`, `TypeRef.Items`). They may not depend on how an OpenAPI document nests schemas, `$ref`s, or content maps. If that document shape changes, only the loader changes.

`internal/ir` must not import kin-openapi types. A `*openapi3.T`, schema, or any other type from that module leaking into the IR would couple every generator to one parser. The loader translates into IR structs built from the standard library alone, so replacing kin-openapi stays inside `internal/loader`.

## IR design decisions and tradeoffs

The IR keeps what a client and a mock server both need, and drops the rest of the OpenAPI document.

**One resolved type.** `TypeRef.Kind` is `primitive`, `object`, `array`, or `enum`. `$ref` is followed during the load, before a `TypeRef` exists, so generators never see a reference. A cycle becomes a named object with no properties, which stops a generator from recursing forever. The cost is that a recursive schema loses its nested shape in the IR. Clients still have a named type to point at.

**Names come from the component.** A schema under `components.schemas` keeps that name, and generators emit a declaration for a named object, array, or enum. An inline schema stays inline. We do not invent names for anonymous objects. The cost is that a large inline object is repeated at every use instead of becoming a shared type.

**`operationId` is required.** Generators turn it into the method name. An operation without one is skipped, with a warning that tells the author to add an `operationId`. Inventing a name from the method and path would collide and would change whenever the path changes.

**Parameters.** `In` is `query`, `path`, or `header`. A path parameter is treated as required even when the spec omits `required: true`, because a path template cannot be called without it. Query and header parameters stay optional unless the spec marks them required. Cookie parameters and parameter styles (`matrix`, `label`, `deepObject`) are not represented. Call sites stay a flat argument list.

**One body, one success response.** A request body keeps a single content type: `application/json` when that media type exists, otherwise the first media type by name. Other media types on the same operation are dropped. The success response is the lowest 2xx. A `default` response is stored with status 0, because the key is not an integer, and it is not chosen as the success body. The mock and the clients speak JSON. A spec whose contract is `text/plain` or `multipart/form-data` will not round-trip.

**Enums are strings.** Enum values are stored with `fmt.Sprint`. A numeric enum is still a list of strings, and a generator decides whether to emit a numeric literal by parsing the text. One field type is simpler than `any`. The original JSON type is recoverable only when the text is a number.

**What we drop on purpose.** `format` (`int32`, `int64`, `date-time`) is not stored. TypeScript `number` and Python `int` do not need it, and we lose a hint about 64-bit integers. `nullable`, `allOf`, `oneOf`, `anyOf`, and discriminators are not in the IR. kin-openapi may flatten some of these during validation, but a union is not a `TypeRef`. Generators stay a straight walk. A spec that needs unions waits until the IR grows a kind for them. Server variables are not substituted. `BaseURL` is the first server URL, or empty when the document has no servers. The generated client takes that string as the default host.

## Why text/template

Each generator walks the IR in Go (`prepare`) and then executes `text/template` for the two output files. The walk decides names, argument order, imports, and the path expression. The template is the shape of the file, with conditionals for optional query parameters, headers, and a request body.

`text/template` is in the standard library. The output is TypeScript and Python, which this repo does not parse. An AST builder for either language would be a second compiler, and a Go-only library such as jennifer cannot emit those files. A general OpenAPI code generator brings its own document model, which we would throw away because we already have `ir.API`.

The cost is that a template can emit source that does not parse. The TypeScript tests run `tsc` when it is installed, and the Python tests import the generated module. Those checks sit next to the generator, so a broken template fails in `go test`.

## Why stdlib net/http for the mock

The mock registers one handler per endpoint on an `http.ServeMux`, using the Go 1.22 pattern `METHOD /path/{param}`. That pattern is the method and path already stored on `ir.Endpoint`. `http.Server` is what `mock` shuts down on interrupt, with a timeout.

chi, echo, and gin add a router, middleware, and a request binder. The mock does not validate incoming requests and does not layer middleware. It returns the lowest 2xx status and a JSON example built from the response schema. The standard library already routes that.

The cost is the same as the scope: no CORS headers, no request validation, no content negotiation beyond the one media type kept in the IR. Conformance checks the response the mock promised, not that the mock rejects a bad request.

## Conformance testing philosophy

Conformance is a contract test of the IR against the mock, shared by every language.

The command generates the client for `--language` first. If generation fails, conformance fails, so a language that cannot emit is caught before any HTTP call. The HTTP checks themselves do not execute the generated TypeScript or Python. They call each endpoint with `net/http` and example data from the same IR the generator walked, then record status, JSON, and content type.

One harness covers TypeScript and Python. CI does not need Node or a Python type checker for this command. The language tests already inspect the generated source and, when the compiler is present, parse it. A bug that lives only in the generated URL encoding will show up there, not in conformance.

The cost is that conformance can pass while a generated client still mis-encodes a call. We take that split on purpose: the mock and the IR are checked once, and each language is checked by reading what it emitted.
