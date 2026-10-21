# SpecForge

SpecForge is a Go CLI that reads an OpenAPI 3.x spec, parses it once into a language-agnostic intermediate representation, and generates typed API clients (TypeScript and Python) and a mock server from that IR. A conformance harness checks that generated clients can call the generated mock server for every endpoint.

## Install

```bash
go install github.com/atharv1730/SpecForge@latest
```

## Usage

Placeholder commands (not implemented yet):

```bash
specforge generate --spec path/to/openapi.yaml --output ./out --language typescript
specforge generate --spec path/to/openapi.yaml --output ./out --language python
specforge mock --spec path/to/openapi.yaml --port 8080
```
