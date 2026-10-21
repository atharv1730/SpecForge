# Design

## Why Cobra

SpecForge is two commands (`generate` and `mock`) with different flag sets. `spf13/cobra` models that as a command tree: each command owns its flags, help text, and required-flag checks. `MarkFlagRequired` is how `--spec`, `--output`, and `--language` fail closed when a caller omits them, without a hand-written presence check per flag.

`urfave/cli` can express the same tree and also has required flags. It is a smaller API. Cobra still wins for this tool because its help output, `--version` handling, and command objects are what we assert against in tests (`SetArgs`, `SetOut`, `Execute`). One well-known shape is cheaper than a second CLI style we would only use here.

The standard library `flag` package parses a single flag set. Subcommands mean a `FlagSet` per command, a manual dispatcher, hand-written help, and hand-written required-flag errors. That is less to import and more to keep correct the moment a third command shows up. The dependency cost of Cobra is one module; the cost of `flag` is owning the CLI framework ourselves.

## Why kin-openapi

The loader runs once. It turns an OpenAPI 3.x document into a typed model, and everything after that walks our IR, not the spec. `github.com/getkin/kin-openapi` is the loader most Go OpenAPI tools already use for that load, resolve, and validate step, so its failure modes are familiar and its API is stable enough to hide behind the IR boundary.

`github.com/pb33f/libopenapi` covers OpenAPI 3.1 more completely and exposes a lower-level AST plus its own index and resolver. We would not keep that AST. The next step collapses the document into the IR, so the extra model is a second thing to learn for a single parse.

The tradeoff we accept: kin-openapi has lagged libopenapi on some 3.1 features (webhooks, JSON Schema constructs that exist only in 3.1, certain keywords beside `$ref`). If a spec we must support hits that gap, the swap stays inside the loader. Until then the smaller, more widely deployed library is the one we parse with.

## IR design philosophy

Parse once, generate many. The loader reads an OpenAPI 3.x document a single time and emits an `ir.API`. The TypeScript client, the Python client, and the mock server all walk that same value. A new language is another walk of the IR, not another parser and not another pass over the spec.

The IR is the contract between the loader and every consumer. Generators may depend on the field names, on `TypeRef.Kind` (`primitive`, `object`, `array`, `enum`), and on which pointers are nil (`Endpoint.RequestBody`, `Response.Schema`, `TypeRef.Items`). They may not depend on how an OpenAPI document nests schemas, `$ref`s, or content maps. If that document shape changes, only the loader changes.

`internal/ir` must not import kin-openapi types. A `*openapi3.T`, schema, or any other type from that module leaking into the IR would couple every generator to one parser. The loader translates into IR structs built from the standard library alone, so replacing kin-openapi stays inside `internal/loader`.

