# AGENTS.md

Guide for coding agents working in `github.com/slinet/ehdb`.
This repo is a single Go module with two binaries: API and sync CLI.

## 1) Scope and Principles
- Make minimal, targeted changes.
- Match existing repository patterns before adding new abstractions.
- Validate with format, vet, test, and build before finishing.
- Avoid introducing JS/TS assumptions (this is a Go project).

## 2) Repository Map
- `cmd/api/main.go`: API entrypoint.
- `cmd/sync/main.go`: sync CLI entrypoint.
- `internal/config`: config structs + Viper loading/defaults.
- `internal/database`: pgx pool + models.
- `internal/handler`: Gin HTTP handlers.
- `internal/middleware`: recovery/error/CORS middleware.
- `internal/crawler`: crawl/sync logic.
- `internal/scheduler`: scheduled sync jobs.
- `internal/logger`: zap logger setup.
- `pkg/utils`: response/sql/parser utility code.

## 3) Authoritative Commands (from justfile)

### Build
- `just build`
- `just build-api`
- `just build-sync`
- Raw equivalents:
  - `go build -o bin/ehdb-api cmd/api/main.go`
  - `go build -o bin/ehdb-sync cmd/sync/main.go`

### Run
- `just run-api`
- `just run-api-scheduler`

### Test
- `just test` (runs `go test -v ./...`)
- `just test-coverage`
- Single package test:
  - `go test -v ./pkg/utils`
- Single test function (important):
  - `go test -v ./pkg/utils -run '^TestParseSearchKeyword$'`
- Generic single-test pattern:
  - `go test -v ./<package-path> -run '^TestName$'`

### Format, Vet, Lint
- `just fmt` -> `go fmt ./...`
- `just vet` -> `go vet ./...`
- `just lint` -> `golangci-lint run` (if installed)
- `just ci` -> `fmt + vet + test`

### Dependencies
- `just deps` -> `go mod download && go mod tidy`
- `just deps-update` -> `go get -u ./... && go mod tidy`

## 4) Coding Style: Imports and Formatting
- Use Go default formatting (`gofmt` / `go fmt`).
- Keep import groups in this order:
  1) standard library
  2) third-party packages
  3) internal module imports (`github.com/slinet/ehdb/...`)
- Keep one blank line between import groups.

## 5) Coding Style: Types and Struct Tags
- Prefer explicit struct types over map-like ad hoc payloads.
- Use `json:"..."` tags for API/model serialization.
- Use `mapstructure:"..."` tags for config structs.
- Use pointer fields (`*string`, `*int`, etc.) for nullable values.
- Keep snake_case JSON field names when already established in models.

## 6) Coding Style: Naming
- Exported symbols: PascalCase.
- Internal/private symbols: camelCase.
- Constructors: `NewXxx`.
- Handler methods: `GetXxx`, `Search`, etc.
- Sync subcommand executors: `runXxx`.

## 7) Error Handling and Logging
- Prefer early-return error handling (`if err != nil { ...; return }`).
- Wrap lower-level errors with context via `%w`:
  - `fmt.Errorf("failed to ...: %w", err)`
- In handlers:
  - invalid input/params -> HTTP 400
  - DB/internal error -> HTTP 500
  - success -> HTTP 200
- Use common response helpers from `pkg/utils/response.go`.
- Use structured zap logs (`zap.String`, `zap.Int`, `zap.Error`).
- Do not swallow panics/errors silently; middleware already handles recovery.

## 8) HTTP/API Conventions
- API routes are under `/api` group in `cmd/api/main.go`.
- Keep existing route style and backward compatibility.
- Preserve response envelope format from utility helpers.
- Preserve cursor pagination behavior where endpoint already supports it.

## 9) Database and SQL Conventions
- Use shared pgx pool from `internal/database`.
- Use context-aware DB calls.
- Use parameterized SQL (`$1`, `$2`, ...), never string interpolation.
- Close query rows with `defer rows.Close()`.
- Keep SQL readable as multiline literals when complex.
- Log DB errors with context; do not expose internals in user-facing messages.

## 10) Testing Conventions
- Use Go `testing` package.
- Prefer table-driven tests for parser/business logic.
- Use `t.Run` with descriptive case names.
- Existing tests commonly compare complex values with `reflect.DeepEqual`.

## 11) Config and Runtime Expectations
- Config is loaded by Viper with defaults in `internal/config/config.go`.
- Keep existing flags stable (`-config`, `-scheduler`, etc.).
- Do not change default runtime behavior unless task explicitly requires it.

## 12) Cursor and Copilot Local Rules
- No `.cursorrules` file found.
- No `.cursor/rules/` directory found.
- No `.github/copilot-instructions.md` file found.
- If such files are added later, treat them as higher-priority local policy.

## 13) Agent Completion Checklist
Run before finishing:
1. `just fmt`
2. `just vet`
3. `just test` (single-test commands are fine during iteration)
4. `just build`
5. Optional: `just lint` when `golangci-lint` is available

## 14) Anti-Patterns to Avoid
- Unrelated refactors in focused fixes/features.
- Breaking response envelope conventions.
- Inconsistent naming/tag style.
- Replacing structured logging with `fmt.Println` in core paths.
- Adding dependencies without clear need and pattern alignment.

## 15) Practical Command Snippets
- Run one package only:
  - `go test -v ./internal/handler`
- Run one test in a package:
  - `go test -v ./internal/handler -run '^TestName$'`
- Build only API binary fast:
  - `go build -o bin/ehdb-api cmd/api/main.go`
- Build only sync binary fast:
  - `go build -o bin/ehdb-sync cmd/sync/main.go`
- Execute all local quality gates quickly:
  - `just ci && just build`
