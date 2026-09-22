# api-mock

Run a local REST mock server from **one JSON file** — pure Go stdlib,
zero dependencies, never touches the network.

```json
{
  "port": 8080,
  "routes": [
    { "method": "GET", "path": "/users/:id",
      "body": {"id": "{{id}}", "name": "ada"} },
    { "method": "POST", "path": "/users", "status": 201, "body": {"ok": true},
      "schema": {"type": "object", "required": ["name"]} }
  ]
}
```

```console
$ api-mock spec.json
api-mock 1.0.0 serving 2 routes from spec.json on http://localhost:8080

$ curl localhost:8080/users/42
{"id": "42", "name": "ada"}

$ curl -X POST -d '{}' localhost:8080/users
{"error":"request body failed schema validation","violations":["body: missing required property \"name\""]}
```

## Features

- **Hot reload** — the spec file is polled; edits go live without a
  restart. A broken edit is logged and the previous routes keep serving.
- **Latency simulation** — global `delayMs` / per-route `delayMs`, plus
  `delayJitterMs` for random extra lag.
- **Request schema validation** — a pragmatic JSON-Schema subset
  (`type`, `required`, `properties`, `items`, `minLength`, `maxLength`,
  `minimum`, `maximum`); violations return `400` with a full list.
- **Path params** — `:param` segments, substituted into response bodies
  with `{{param}}` placeholders.
- **Strict spec parsing** — unknown fields, duplicate routes, missing
  bodies and bad statuses are rejected at load time, not at request time.

## Usage

```
api-mock <spec.json> [options]

  -addr <host:port>   listen address (default: spec "port" or :8080)
  -validate           validate the spec and exit 0/1 — handy in CI
  -watch-interval <d> hot-reload poll interval (default: 500ms)
  -h / -V             help / version
```

Flags may come before or after the spec path
(`api-mock spec.json -validate` works).

Response bodies are inline JSON (`body`) or files next to the spec
(`bodyFile`). `status` defaults to 200; `204`/`304` need no body.

## Exit codes

| Code | Meaning |
|---|---|
| 0 | success |
| 1 | spec invalid / unreadable, listen failure, `-validate` failure |
| 2 | usage error |

## Development

```console
$ go test ./...          # unit + httptest end-to-end + hot reload
$ go test -race ./...
$ gofmt -l . && go vet ./...
```

## License

MIT — see [LICENSE](LICENSE).
