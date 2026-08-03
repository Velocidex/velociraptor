# VQL LSP Server — Test Summary

This document summarizes every test performed on the VQL language server
(`velociraptor lsp`) across the prototype milestones: the participle v2
migration, the Inspect() and Outline() APIs, the diagnostic engine, the
artifact registry, the opencode integration, and document symbols.

**Go unit test total:** 29 tests in `vql/lsp/` (10 diagnostic engine + 6
server lifecycle + 2 document symbols + 5 hover + 6 completion), plus 2
vfilter Inspect() tests, 2 vfilter Outline() tests, and the full vfilter
migration suite.

---

## 1. vfilter unit tests (participle v2 migration)

Location: `~/projects/vfilter` — branch `participle-v2-upgrade`
(commits `abfe2d4`, `95a1f79`; Inspect API uncommitted)

Command: `go test ./...` — **all 8 packages pass**:

| Package | Result |
|---|---|
| vfilter | ok |
| arg_parser | ok |
| benchmarks | ok |
| explain | ok |
| marshal | ok |
| reformat | ok |
| scope | ok |
| utils | ok |

### Migration regression tests (pre-existing suite, all green after upgrade)

- `TestVQLQueries` / `TestMultiVQLQueries` — golden-file parse tests. Caught
  the comment-elision regression: v2's parser-level `Elide` no longer drops
  tokens at the lexer, so comments leaked into the AST. Fixed with a
  `droppingLexerDef` wrapper.
- `TestMultiVQLQueries` (multiline string) — caught missing `(?s)` flag on the
  `MultilineString` lexer rule.
- `TestSerializaition` / `TestVQLSerializaition` — caught position metadata
  polluting `cmp.Diff` after adding `Pos`/`EndPos`; fixed with
  `compareOptionsWithPos` ignoring `lexer.Position`.

### Position capture verification (manual demo)

`/tmp/opencode/posdemo/` — parses
`SELECT Foo, Bar(X=1) FROM Artifact.Linux.Sys() WHERE Foo > 3` and prints
AST node spans. Verified every node maps exactly to its source offset:

| Node | Expected | Actual |
|---|---|---|
| statement / SELECT | 1:1–1:61 | 1:1–1:61 |
| plugin `Artifact.Linux.Sys` | 1:27–1:48 | 1:27–1:48 |
| column `Foo` | 1:8–1:11 | 1:8–1:11 |
| column `Bar(X=1)` | 1:13–1:22 | 1:13–1:22 |

---

## 2. vfilter Inspect() API tests

Location: `~/projects/vfilter/inspect_test.go` — **2 tests, both pass**

| Test | What it verifies |
|---|---|
| `TestInspect` | Parses `LET Y = 5\nSELECT Foo(X=1), Bar FROM Artifact.Linux.Sys.Users() WHERE Foo > 3 AND baz(X=Y)`; asserts 1 Let (Y), 3 calls (plugin `Artifact.Linux.Sys.Users` with IsPlugin=true, `Foo` with arg X, `baz`), symbols include Y and Foo, and the plugin's Pos is at 2:27. |
| `TestInspectNil` | `Inspect(nil)` is nil-safe (no panic). |

---

## 3. LSP diagnostic engine unit tests

Location: `velociraptor/vql/lsp/diagnostics_test.go` — **10 tests, all pass**

Command: `go test ./vql/lsp/`

Test registry (fake): plugins `pslist` (arg `pid`), `Artifact.Linux.Sys.Users`;
function `upcase` (arg `str`).

| Test | Query | Expected diagnostics |
|---|---|---|
| `TestValidateCleanDocument` | `SELECT * FROM pslist(pid=1) WHERE Name =~ 'foo'` | none |
| `TestValidateUnknownFunction` | `SELECT upcase(str='x'), unknownfunc() FROM pslist()` | 1: `Unknown function 'unknownfunc'` at 0:24 |
| `TestValidateUnknownPlugin` | `SELECT * FROM bogusplugin()` | 1: `Unknown plugin 'bogusplugin'` at 0:14 |
| `TestValidateUnknownArgument` | `SELECT * FROM pslist(foo=1)` | 1: `Unknown argument 'foo' for plugin 'pslist'` at 0:21 |
| `TestValidateMultilinePositions` | bogus function on line 2 | diagnostic at line 1 col 24 (0-based 1:24) |
| `TestValidateSyntaxErrorOnly` | `SELECT * FROM` | 1 diag containing "unexpected token" |
| `TestValidateTruncateAndRetry` | 4-line doc: bad arg on line 1, `@@@` garbage on line 2, bad arg line 3, bogus plugin line 4 | **all 4**: foo arg (0:21), `lexer: invalid input text "@@@"` (1:7), bar arg (2:21), `bogusplugin` (3:14) |
| `TestValidateTruncateAndRetrySyntaxErrorPosition` | 2 good lines then `SELECT @@@` | 1 diag at line 2 col 7 |
| `TestValidateArtifactArgument` | `Artifact.Windows.Sys.Users()` (via `AddArtifact`, arg `remoteRegKey`) | valid call clean; `(foo=1)` → `Unknown argument 'foo' for artifact 'Artifact.Windows.Sys.Users'` at Character 41 |
| `TestValidateKnownArtifactParameters` | `Artifact.Generic.Client.VQL(Command='SELECT 5')` (arg `Command`) | clean |

Key behaviors proven:

- Clean documents produce zero diagnostics.
- Unknown functions, plugins, and keyword arguments are each reported with
  exact byte-based ranges (0-based LSP positions).
- Syntax errors are reported with precise position from the underlying
  `*lexer.Error` / `participle.Error`.
- **Truncate-and-retry**: a syntax error mid-document does NOT suppress
  diagnostics from valid statements before or after it.
- Multi-line documents map positions to the correct line.

---

## 3b. LSP server lifecycle unit tests

Location: `velociraptor/vql/lsp/server_test.go` — **6 tests, all pass**

Uses a `mockClient` (embeds `protocol.UnimplementedClient`, records published
diagnostics per URI) so no real stdio transport is needed.

| Test | What it verifies |
|---|---|
| `TestServerInitializeCapabilities` | `initialize` returns full-sync `TextDocumentSync`, `DiagnosticProvider` (union — type-asserted to `*protocol.DiagnosticOptions`) with identifier `"vql"`, and `ServerInfo.Name == "vql-lsp"` |
| `TestServerDidOpenAndPullDiagnostic` | After `didOpen` of a bad doc, pull `Diagnostic()` returns the correct item AND push diagnostics fired |
| `TestServerDidChangeUpdatesDocument` | Full-sync `didChange` replaces the document; subsequent pull reports the new error |
| `TestServerDidCloseClearsDiagnostics` | `didClose` clears both the pull report and the pushed diagnostics |
| `TestServerPullUnknownDocument` | Pull on a never-opened URI returns empty items (no crash) |
| `TestServerShutdownClosesDone` | `Shutdown` closes the server's `Done()` channel |

## 3c. Document symbol unit tests

Location: `velociraptor/vql/lsp/server_test.go` + `~/projects/vfilter/outline_test.go`

### vfilter Outline() API — 2 tests, all pass

| Test | What it verifies |
|---|---|
| `TestOutline` | `Outline()` produces a hierarchical outline of `LET Y = SELECT Foo FROM pslist(pid=1)\nSELECT upcase(str=X), Bar AS baz FROM Artifact.Linux.Sys.Users() WHERE Foo > 3`: stmt1 root `Y` (let) → `pslist` (query) → `Foo` (column); stmt2 root `Artifact.Linux.Sys.Users` (query) → 2 columns (unaliased `""` with `upcase` function child, aliased `"baz"`) |
| `TestOutlineNil` | `Outline(nil)` returns nil (no panic) |

### LSP server DocumentSymbol — 2 tests, all pass (total 18 in vql/lsp/)

| Test | What it verifies |
|---|---|
| `TestServerDocumentSymbols` | After `didOpen`, `DocumentSymbol` returns the hierarchical outline: `Y` (Variable) → `pslist` (Function) → `Foo` (Field); `Artifact.Linux.Sys.Users` (Function) → unaliased column (Field, name from source text) → `upcase` (Function), and `baz` (Field) |
| `TestServerDocumentSymbolsUnknownDocument` | `DocumentSymbol` on a never-opened URI returns empty (no crash) |

Kind mapping verified: `let→Variable(13)`, `query/function→Function(12)`,
`column→Field(8)`. `Initialize` capabilities now advertise
`DocumentSymbolProvider: true`.

## 3d. Hover unit tests

Location: `velociraptor/vql/lsp/server_test.go` — 5 tests, all pass
(total 23 in vql/lsp/)

| Test | What it verifies |
|---|---|
| `TestServerHoverPluginCall` | Cursor on `pslist` in `SELECT * FROM pslist(pid=1)` → markdown contains `**pslist** (plugin)` and `pid` |
| `TestServerHoverFunctionCall` | Cursor on `upcase` → contains `**upcase** (function)` and `str` |
| `TestServerHoverArgument` | Cursor on `pid` arg → contains `**pid**` and `int` |
| `TestServerHoverUnknownDocument` | Never-opened URI → nil (no crash) |
| `TestServerHoverUnknownSymbol` | Cursor on a dynamic column (`SomeDynamicColumn`) → nil (no false positive) |

Byte-offset gotcha: cursor positions are computed per document string
(e.g. `pslist` is at byte 14 in `SELECT * FROM pslist(pid=1)`).

## 3e. Completion unit tests

Location: `velociraptor/vql/lsp/server_test.go` — 6 tests, all pass
(total 29 in vql/lsp/)

| Test | What it verifies |
|---|---|
| `TestServerCompletionPluginPrefix` | `psl` prefix → labels contain `pslist` |
| `TestServerCompletionFunctionPrefix` | `upc` prefix → labels contain `upcase` |
| `TestServerCompletionArtifactPrefix` | `Art` prefix → labels contain `Artifact.Linux.Sys.Users` |
| `TestServerCompletionLetVariable` | After `LET X = 5\nSELECT * FROM ` → some item `X` with kind Variable |
| `TestServerCompletionArguments` | Inside `pslist(` → some item `pid` with kind Field |
| `TestServerCompletionUnknownDocument` | Never-opened URI → empty labels |

LET-variable recovery gotcha: when the document ends in an incomplete
statement (whole-document parse fails), `letNames` falls back to
`largestParseablePrefix` so LET variables are still offered.

`Initialize` capabilities advertise `CompletionProvider` with
`TriggerCharacters: [".", "("]`.

---

## 4. End-to-end tests against the real binary

### 4a. `velociraptor lsp --check` (CLI smoke tests)

Binary: `~/.local/bin/velociraptor` (also built ad-hoc to
`/tmp/opencode/velociraptor-lsp` during development).

| Query | Result |
|---|---|
| `SELECT * FROM pslist(pid=1) WHERE Name =~ 'foo'` | clean (no output) |
| `SELECT upcase(str='x'), * FROM pslist(foo=1) WHERE unknownfunc()` | 3 diags: arg `str` on upcase (1:15), arg `foo` on pslist (1:39), unknown function (1:48) |
| `SELECT * FROM` | syntax diag: `unexpected token "<EOF>"` at 1:14 |
| `SELECT * FROM pslist(badarg=1)` | `Unknown argument 'badarg' for plugin 'pslist'` at 1:22 |
| `SELECT * FROM Artifact.Windows.Sys.Users()` | clean (artifact resolved from repository) |
| `SELECT * FROM Artifact.Windows.Sys.Users(foo=1)` | `Unknown argument 'foo' for artifact ...` at 1:42 |
| `SELECT * FROM Artifact.Bogus.Nope()` | `Unknown plugin 'Artifact.Bogus.Nope'` |
| `SELECT * FROM Artifact.Generic.Client.VQL(Command='SELECT 5')` | clean |
| `Artifact.Generic.Client.VQL(Command='SELECT 5', badparam=1)` | `Unknown argument 'badparam' for artifact ...` at 1:63 |
| `SELECT * FROM\` (EOF) | diag at line 2 col 14, `unexpected token <EOF>` |

### 4b. Full LSP protocol tests (Python stdio clients)

Python clients in `/tmp/opencode/` (`lsp_client.py`, `lsp_change.py`,
`lsp_pull_test.py`, `kick_test.py`) speak Content-Length framed LSP over
stdio.

Verified:

- **Handshake**: `initialize` returns capabilities
  `{textDocumentSync: 1, diagnosticProvider: {identifier: "vql",
  interFileDependencies: false, workspaceDiagnostics: false}}` and
  serverInfo `{name: "vql-lsp", version: "0.1"}`.
- **Document sync**: `didOpen`/`didChange` (full-sync) update the document;
  `didClose` clears diagnostics.
- **Push diagnostics**: `textDocument/publishDiagnostics` fires on open/change
  with the correct diagnostic list.
- **Pull diagnostics**: `textDocument/diagnostic` returns a
  `RelatedFullDocumentDiagnosticReport` with `kind: "full"` and the same
  diagnostics (LSP 3.17 style, used by opencode).
- **Lifecycle**: `shutdown` → `{"result": null}`, `exit` → process exits 0.
- A bad doc (`Artifact.Windows.Sys.Users(foo=1)`) returns
  `Unknown argument 'foo' for artifact 'Artifact.Windows.Sys.Users'` at
  0:41 through BOTH push and pull channels.
- **Document symbols**: `textDocument/documentSymbol` returns the
  hierarchical outline for a two-statement doc
  (`LET Y = SELECT Foo FROM pslist(pid=1)\nSELECT upcase(str=X), Bar AS baz FROM Artifact.Linux.Sys.Users() WHERE Foo > 3`):
  `Y` (Variable) → `pslist` (Function) → `Foo` (Field); `Artifact.Linux.Sys.Users`
  (Function) → `upcase(str=X)` (Field, name from source) → `upcase` (Function),
  and `baz` (Field). (Python client `/tmp/opencode/lsp_symbols_test.py`; note
  the push notification arrives before the response — drain loop required.)
- **Hover**: `textDocument/hover` on `upcase` in
  `SELECT upcase(str='x') FROM pslist(pid=1)` returns markdown with the
  REAL registry doc — `**upcase** (function)` + "Returns the uppercase
  version of a string." + argument `string` (real name, not `str`); hover
  on the `pid` argument returns `**pid** — int64` (real type). Capabilities
  advertise `hoverProvider: true`. (Python client
  `/tmp/opencode/lsp_hover_test.py`.)
- **Completion**: `textDocument/completion` with prefix `psl` → `pslist`;
  `upc` → `upcase`; `Art` → full artifact tree
  (`Artifact.ADX.Flows.Upload`, `Artifact.Admin.Client.*`, ...); LET
  variable `X` after `SELECT * FROM `; argument `pid` inside `pslist(`.
  Capabilities advertise `completionProvider` with trigger characters
  `.` and `(`. (Python client `/tmp/opencode/lsp_completion_test.py`;
  ALL OK — see the test file's own output for the 5-case table.)

---

## 5. opencode integration test (the "kick the tires" test)

- Config: `/home/me/.config/opencode/opencode.json` — custom LSP server
  `{"command": ["velociraptor", "lsp"], "extensions": [".vql"]}`.
- Binary deployed to `~/.local/bin/velociraptor` (on PATH).
- After restarting opencode and opening `lsp-test/test.vql`, the server
  process was observed running (`velociraptor lsp`, PID observed, CPU
  settling after startup).
- **Test file `lsp-test/broken.vql`** containing five deliberate errors:

  ```
  SELECT upcase(str='x'), bogusfunc() FROM Artifact.Windows.Sys.Users(foo=1)
  SELECT * FROM Artifact.Bogus.Nope()
  SELECT @@
  ```

  Writing the file produced the following diagnostics injected directly into
  the agent's tool result (opencode agent feedback loop):

  ```
  ERROR [3:8] lexer: invalid input text "@@\n"
  ERROR [1:15] Unknown argument 'str' for function 'upcase'
  ERROR [1:25] Unknown function 'bogusfunc'
  ERROR [1:69] Unknown argument 'foo' for artifact 'Artifact.Windows.Sys.Users'
  ERROR [2:15] Unknown plugin 'Artifact.Bogus.Nope'
  ```

  All five error classes detected, including diagnostics both before AND
  after the line-3 syntax error (truncate-and-retry working in the real
  client). The `str` diagnostic was cross-checked against
  `velociraptor vql list`: `upcase` genuinely takes `string`, not `str` —
  confirmed a true positive.

---

## 6. Agent-discovery option tests (see VQL-LSP-USAGE.md)

### 6a. `validate-vql` custom opencode tool (Option 2)

The custom tool at `~/.config/opencode/tools/validate-vql.ts` shells out to
`velociraptor lsp --check` and parses the output into structured JSON.
After an opencode restart, the tool appears in the agent's function list.
Verified from inside the live agent:

| Query | Result |
|---|---|
| `SELECT upcase(str='x'), bogusfunc() FROM Artifact.Windows.Sys.Users(foo=1)` | `valid: false`, 3 diagnostics (1:15 str/upcase, 1:25 bogusfunc, 1:69 foo/artifact) |
| `SELECT * FROM Artifact.Windows.Sys.Users()` | `valid: true`, diagnostics [] |
| `SELECT * FROM pslist(pid=1)` | `valid: true`, diagnostics [] |

Also `bun build tools/validate-vql.ts` compiles cleanly (74 modules,
0.42 MB).

### 6b. velociraptor-vql-validation skill (Option 1)

Skill file created at
`~/.config/opencode/skills/velociraptor-vql-validation/SKILL.md`; its
quick-start command (`velociraptor lsp --check`) is the same path proven in
sections 4 and 6a. Skill frontmatter auto-matches when a task mentions
writing/checking/debugging VQL.

### 6c. mcpls LSP→MCP bridge (Option 3a)

mcpls v0.3.8 (rebuilt from source — prebuilt binary required glibc 2.39,
system has 2.35) wraps the vql LSP server and exposes MCP tools. Verified
with a newline-delimited-JSON MCP client:

- initialize → protocolVersion 2024-11-05, serverInfo {name mcpls, title
  "MCPLS - MCP to LSP Bridge", version 0.3.8}.
- tools/list → 20 tools including `get_diagnostics`.
- `tools/call get_diagnostics` on
  `/home/me/projects/velociraptor/lsp-test/bridge-test.vql` (2-line bad
  file) returned both diagnostics as JSON:
  `Unknown argument 'foo' for artifact 'Artifact.Windows.Sys.Users'`
  (1:42) and `Unknown argument 'badarg' for plugin 'pslist'` (2:22).

Config in `/home/me/.config/mcpls/mcpls.toml` (roots restricted to
`/home/me/projects/velociraptor`; `[[workspace.language_extensions]]` for
vql; `[[lsp_servers]]` entry running `velociraptor lsp` with
`handles = ["diagnostics"]`).

### 6d. Native MCP tool `validate_vql` in mcp-velociraptor (Option 3b)

Tool implemented in `/home/me/projects/mcp-velociraptor` at
`internal/tools/validate_vql.go` (lazy cached registry via
`lsp.BuildRegistryWithArtifacts`; no API connection required for
validation itself, though the server requires an API config at startup).
Verified with a newline-delimited-JSON MCP client both standalone and
against a **live Velociraptor instance** (API config at
`/velociraptor/datastore5/api.config.yaml`, `localhost:8001`):

- tools/list → [`list_clients`, `validate_vql`].
- `validate_vql` bad query → `valid: false`, 3 diagnostics (1:15, 1:25,
  1:69) exactly matching the LSP/CLI/tool results.
- `validate_vql` good query → `valid: true`, diagnostics [].
- `list_clients` against the live instance returned 3 real clients
  (hostname `1oca1host`, linux linuxmint21.3), proving the shared registry
  and API connection work together.

---

## Summary of coverage

| Error class | Unit test | CLI test | LSP test | opencode |
|---|---|---|---|---|
| Syntax error | ✓ | ✓ | ✓ | ✓ |
| Unknown function | ✓ | ✓ | ✓ | ✓ |
| Unknown plugin | ✓ | ✓ | ✓ | ✓ |
| Unknown keyword arg (plugin) | ✓ | ✓ | ✓ | — |
| Unknown keyword arg (artifact) | ✓ | ✓ | ✓ | ✓ |
| Unknown artifact | ✓ | ✓ | — | ✓ |
| Multi-line position accuracy | ✓ | — | — | ✓ |
| Truncate-and-retry (errors after syntax error) | ✓ | ✓ | — | ✓ |
| Clean document → no diagnostics | ✓ | ✓ | ✓ | ✓ |
| Pull-based diagnostics | — | ✓ | ✓ | ✓ |
| Push diagnostics on open/change | — | ✓ | ✓ | — |
| didClose clears diagnostics | — | ✓ | ✓ | — |
| Shutdown/exit lifecycle | — | ✓ | ✓ | — |
| Initialize capabilities (full sync, provider, serverInfo) | — | ✓ | ✓ | ✓ |
| Document symbols (hierarchical outline, kinds) | — | ✓ | ✓ | — |
| Hover (function/plugin/argument docs + types) | — | ✓ | ✓ | — |
| Completion (plugins, functions, artifacts, LET vars, args) | — | ✓ | ✓ | — |

---

## Artifact registry facts discovered during testing

- The base scope (from `vql_subsystem.MakeScope()`) contains ~437 built-in
  plugins/functions but NOT artifacts.
- The repository stores artifacts WITHOUT the `Artifact.` prefix
  (e.g. `Windows.Sys.Users`); VQL references them WITH the prefix. The LSP
  registry prepends `Artifact.` when registering.
- Real artifact parameter examples: `Generic.Client.Info` → 0 params,
  `Windows.Sys.Users` → 1 (`remoteRegKey`), `Generic.Client.VQL` → 1
  (`Command`).
