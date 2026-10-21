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

