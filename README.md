# SpecForge

SpecForge is a Go CLI that reads an OpenAPI 3.x document once, turns it into a language-agnostic intermediate representation, and emits typed API clients and a mock server from that model. A conformance command generates a client and checks every endpoint against the mock.

Operations need an `operationId`. An operation without one is skipped.

## Architecture

```
OpenAPI 3.x spec
        |
        v
  loader.Load          parse and validate (kin-openapi)
        |
        v
  loader.Transform     resolve refs into plain Go structs
        |
        v
      ir.API            the only model generators and the mock share
       /    \
      /      \
     v        v
 generators   mock server          TypeScript and Python clients
 (typescript, (net/http)           and example JSON responses
  python)
      \        /
       v      v
     conformance            generate the client, then call every endpoint
```

`internal/ir` does not import the OpenAPI parser. Replacing the parser stays inside `internal/loader`. Adding a language is another walk of `ir.API`.

## Install

```bash
go install github.com/atharv1730/SpecForge@latest
```

The mock server routes with the standard library, so there is no extra router to install.

## Usage

Generate a client. `--language` is `typescript` or `python`. `--output` is the directory the files are written to.

```bash
specforge generate --spec path/to/openapi.yaml --output ./out --language typescript
specforge generate --spec path/to/openapi.yaml --output ./out --language python
```

TypeScript writes `out/types.ts` and `out/client.ts` (a `fetch` client). Python writes `out/models.py` and `out/client.py` (a `urllib.request` client, no third-party packages).

Serve example responses for every operation. `--port` defaults to 8080. The process runs until interrupted.

```bash
specforge mock --spec path/to/openapi.yaml --port 8080
```

Check the spec against the mock. Conformance generates the client for `--language`, starts the mock on `--port`, and calls every endpoint. It checks the status code, that a declared body is JSON, and that the content type matches the spec. The process exits 0 when every check passes and 1 when any fail.

```bash
specforge conformance --spec path/to/openapi.yaml --language typescript --port 8080
specforge conformance --spec path/to/openapi.yaml --language python --port 9090
```

`--verbose` is a global flag. It prints each stage to stderr: the spec being loaded, the document title and version, each endpoint discovered while transforming, and each file written during generation.

```bash
specforge generate --spec path/to/openapi.yaml --output ./out --language typescript --verbose
```

`specforge --help` lists the commands. `specforge --version` prints the version.

## Supported languages

| Language   | `--language` | Files written              | Client runtime                          |
| ---------- | ------------ | -------------------------- | --------------------------------------- |
| TypeScript | `typescript` | `types.ts`, `client.ts`    | `fetch`, no runtime dependencies        |
| Python     | `python`     | `models.py`, `client.py`   | `urllib.request`, no third-party packages |

## Layout

| Path                    | Role                                              |
| ----------------------- | ------------------------------------------------- |
| `cmd/`                  | `generate`, `mock`, and `conformance`            |
| `internal/loader`       | Read an OpenAPI file and build the IR            |
| `internal/ir`           | Language-agnostic model                          |
| `internal/gen`          | TypeScript and Python generators                 |
| `internal/mock`         | Example server                                   |
| `internal/conformance`  | Call every endpoint and record failures          |
| `testdata/petstore.yaml`| Spec used by the tests                           |

Design notes for the parser, the IR, the templates, the mock server, and conformance are in [DESIGN.md](DESIGN.md).
