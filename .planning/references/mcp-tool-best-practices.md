# MCP tool definition best practices

Grounded in Anthropic's engineering post ("Writing effective tools for agents"), the official API docs (`platform.claude.com/docs/en/agents-and-tools/tool-use/define-tools`), and the MCP 2025-11-25 spec. Tools are not API contracts — they are prompts to a non-deterministic agent. Every word in a definition is prompt engineering.

---

## 1. What to build — before writing a single definition

### Don't wrap every endpoint. Build for agent affordances.

Agents have limited context. A tool that returns all 2,000 contacts wastes context; a tool that searches returns only what's relevant. The question is not "what does the API support?" but "what high-impact workflows will agents run?"

Build a few targeted tools first. Each must have a clear, distinct purpose. Overlapping tools or too many tools actively distract agents from efficient strategies.

### Consolidate operations — tools can wrap multiple API calls

Tools can handle multi-step workflows internally, enriching responses with related metadata so the agent doesn't have to chain three calls to get one answer.

| ❌ Granular wrap | ✅ Intent-based tool |
|---|---|
| `list_users` + `list_events` + `create_event` | `schedule_event` (finds availability and books internally) |
| `get_customer_by_id` + `list_transactions` + `list_notes` | `get_customer_context` (compiles all relevant info in one call) |
| `read_logs` (returns all) | `search_logs` (returns only relevant lines with context) |

---

## 2. Naming

### Rule: verb-noun, snake_case, unambiguous

Tool names must match `^[a-zA-Z0-9_-]{1,64}$`. Use verb-noun pairs. No abbreviations.

| ❌ Avoid | ✅ Prefer |
|---|---|
| `calc_sum` | `calculate_sum` |
| `get` | `get_customer_context` |
| `process_it` | `process_payment` |
| `search` | `search_logs` |

### Namespace by service when you have multiple tools

When agents access several MCP servers simultaneously, namespacing prevents selection ambiguity. Choose prefix or suffix consistently — measure it against your actual model, as the effect is non-trivial and varies by LLM.

```
asana_projects_search
asana_users_search
slack_send_message
slack_search_messages
```

---

## 3. Descriptions — the single most important factor

This is by far the highest-leverage thing you can do, per Anthropic's own docs. A 2025 ecosystem study found 97% of MCP tool descriptions contain at least one quality issue; 56% have unclear purpose statements.

### Write for a new hire, not a type checker

The description is injected verbatim into the agent's context at selection time. Target 3–4 sentences minimum; more for complex tools. Make implicit context explicit: specialised query formats, niche terminology, relationships between resources, what the tool does *not* return, when not to use it.

**❌ Too sparse:**
```
"Gets weather"
```

**✅ Explicit and complete:**
```
"Returns current weather conditions for a given city. Use for
current state queries — not forecasts. Returns temperature in
Celsius, humidity %, and a condition string (e.g. 'partly cloudy').
City name must be in English. Do not use for historical weather
data — use get_weather_history instead."
```

### Include when-not-to-use guidance

If two tools look similar, the description must disambiguate them explicitly. Don't assume the model will infer the difference from the name alone.

### Use `input_examples` for complex inputs (not a description substitute)

The API supports an optional `input_examples` array on tool definitions. Use it for tools with nested objects, optional parameters, or format-sensitive inputs. Each example must be schema-valid. Cost: ~20–50 tokens per example.

```go
// In mcp-go, use WithToolInputExamples() if available,
// or supply via raw tool definition
```

---

## 4. Schema — use JSON Schema's full expressiveness

### Express constraints in schema, not just descriptions

Every constraint that can be expressed in JSON Schema should be. Descriptions are invisible to automated tooling and testing pipelines. A real-world audit of popular MCP servers found mutual-exclusivity constraints and numeric bounds documented only in prose — causing silent failures in the field.

| Constraint type | JSON Schema keyword |
|---|---|
| Fixed value sets | `enum` |
| Numeric bounds | `minimum` / `maximum` |
| String patterns | `pattern`, `minLength`, `maxLength` |
| Format hints | `format: "date-time"`, `format: "uri"` |
| Mutual exclusivity | `oneOf` / `anyOf` |
| Required fields | explicit `required` array |

### Unambiguous parameter names — no naked `user`, `date`, `id`

Vague parameter names force the model to guess. Every parameter name should be self-explanatory without reading the description.

| ❌ Ambiguous | ✅ Unambiguous |
|---|---|
| `user` | `user_id` |
| `date` | `start_date_iso8601` |
| `id` | `customer_id` |
| `limit` | `max_results` |

### Full example in mcp-go (mark3labs)

```go
mcp.NewTool("search_logs",
    mcp.WithDescription(
        "Search application logs within a time range. Returns matching "+
            "lines sorted newest-first with ±3 lines of context. Use for "+
            "debugging errors or tracing requests. Do not use for metrics — "+
            "use get_metrics instead. Example: search last hour for "+
            "'timeout' in the api service.",
    ),
    mcp.WithString("query",
        mcp.Required(),
        mcp.Description("Case-insensitive substring to match in log lines."),
    ),
    mcp.WithString("service",
        mcp.Required(),
        mcp.Description("Service to search. Must be one of: api, worker, scheduler."),
        mcp.Enum("api", "worker", "scheduler"),
    ),
    mcp.WithString("level",
        mcp.Description("Minimum log level to include. Defaults to 'error'."),
        mcp.Enum("debug", "info", "warn", "error"),
    ),
    mcp.WithNumber("max_results",
        mcp.Description("Max lines to return. Between 1 and 500. Defaults to 50."),
        mcp.Min(1),
        mcp.Max(500),
    ),
)
```

---

## 5. Annotations — behavioural hints for clients

Added in MCP spec 2025-03-26. Five fields on every tool definition. These are hints for client UX — confirmation dialogs, auto-approval policies, risk indicators. They are **not** enforced security guarantees; a malicious server could lie. Clients must treat them as informational only.

| Field | Default | Meaning |
|---|---|---|
| `title` | — | Human-readable display name for UIs |
| `readOnlyHint` | `false` | Tool does not modify any state |
| `destructiveHint` | `true` | Tool may delete, overwrite, or make irreversible changes |
| `idempotentHint` | `false` | Same inputs → no additional effect on repeated calls |
| `openWorldHint` | `true` | Tool reaches outside its own process (external APIs, internet) |

**Notes on `destructiveHint`:** "Destructive" is broader than deleting rows — it includes revoking tokens, closing issues, overwriting files. If the operation can't be easily undone, mark it destructive.

**Note on `idempotentHint` vs `destructiveHint`:** A tool can be non-idempotent (sending a duplicate email) without being destructive. They are independent axes.

**Only `readOnlyHint: false` makes `destructiveHint` and `idempotentHint` meaningful.** Read-only tools are by definition non-destructive.

### Decision table

```go
// DB query, file read
ReadOnlyHint: mcp.ToBoolPtr(true)
OpenWorldHint: mcp.ToBoolPtr(false)

// Create a record (additive, not reversible without effort)
ReadOnlyHint: mcp.ToBoolPtr(false)
DestructiveHint: mcp.ToBoolPtr(false)
IdempotentHint: mcp.ToBoolPtr(false)
OpenWorldHint: mcp.ToBoolPtr(false)

// Delete a record
ReadOnlyHint: mcp.ToBoolPtr(false)
DestructiveHint: mcp.ToBoolPtr(true)
IdempotentHint: mcp.ToBoolPtr(true)   // deleting twice = same outcome
OpenWorldHint: mcp.ToBoolPtr(false)

// Send email / call external API
ReadOnlyHint: mcp.ToBoolPtr(false)
DestructiveHint: mcp.ToBoolPtr(false)
IdempotentHint: mcp.ToBoolPtr(false)  // sending twice = two emails
OpenWorldHint: mcp.ToBoolPtr(true)
```

---

## 6. Tool responses — return signal, not dump

### Return only high-signal fields

Drop opaque internals. Prefer human-readable identifiers over UUIDs — resolving alphanumeric UUIDs to names measurably reduces hallucinations in retrieval tasks.

| ❌ Low signal | ✅ High signal |
|---|---|
| `uuid: "a1b2c3d4-..."` | `name: "Wang Jun"` |
| `256px_image_url: "..."` | `image_url: "https://..."` |
| `mime_type: "image/jpeg"` | `file_type: "jpeg"` |
| `internal_shard_id: 42` | `department: "Engineering"` |

### Expose a `response_format` enum for verbosity control

When agents need IDs for downstream tool chaining but not every call, let them choose. The Anthropic engineering post showed a Slack tool returning 72 tokens in concise mode vs 206 in detailed — the agent uses concise by default and switches to detailed only when it needs IDs for chaining.

```go
mcp.WithString("response_format",
    mcp.Description(
        "Controls verbosity. 'concise' returns names and content only "+
            "(default). 'detailed' includes IDs required for downstream "+
            "tool calls like reply_to_thread.",
    ),
    mcp.Enum("concise", "detailed"),
)
```

### Truncate with guidance, paginate with cursors

Never silently truncate. Tell the agent what happened and how to get more. Use pagination cursors for list tools — not page numbers, which require the agent to track state.

```json
{
  "results": ["..."],
  "truncated": true,
  "total_count": 847,
  "next_cursor": "eyJvZmZzZXQiOjUwfQ",
  "hint": "Results truncated at 50. Pass next_cursor to get more, or narrow with filter_by_service or filter_by_level."
}
```

---

## 7. Error responses — teach the agent to recover

### Critical distinction: protocol errors vs tool call errors

MCP protocol-level errors (malformed JSON-RPC, method not found) are captured and discarded by the client — the model never sees them. Tool call errors using `isError: true` in the response are injected back into the context window — the model can read and recover from them. This means your error messages are themselves prompts.

### The three-part error template

For validation and business logic failures:

1. **What went wrong** — specific field and value that failed
2. **What was expected** — valid values or format
3. **Example of correct input** — concrete and copy-pasteable

**❌ Opaque:**
```
ValidationError: ERR_042
at handler.go:187
invalid param
```

**✅ Actionable:**
```json
{
  "isError": true,
  "content": [{
    "type": "text",
    "text": "'service' value 'payments' is not valid. Valid values: api, worker, scheduler. Example: service='api'"
  }]
}
```

### For transient failures, include retry strategy

```json
{
  "isError": true,
  "content": [{
    "type": "text",
    "text": "Upstream log service timeout. Retry once immediately. If the error persists, use get_recent_events as a fallback for events from the last 15 minutes."
  }]
}
```

### Catch all panics inside handlers

An unhandled panic crashes the handler and leaves the agent with an opaque transport-level error it cannot recover from. Every tool handler must catch all exceptions and return a structured error response.

```go
func myHandler(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
    defer func() {
        if r := recover(); r != nil {
            // log internally, return clean error to agent
        }
    }()
    // ... handler logic
}
```

---

## Quick reference checklist

Before shipping a tool definition:

- [ ] Does the description say what it does, when to use it, when NOT to use it, and what it returns?
- [ ] Is it at least 3–4 sentences?
- [ ] Are all fixed value sets expressed as `enum`, not just described in prose?
- [ ] Are numeric bounds expressed in schema (`minimum`/`maximum`)?
- [ ] Are all parameter names unambiguous without reading the description?
- [ ] Are all actually-required fields in the `required` array?
- [ ] Are annotations set correctly (`readOnlyHint`, `destructiveHint`, `openWorldHint`)?
- [ ] Does the response omit opaque internals and UUIDs where possible?
- [ ] Does the response truncate with a `hint` on how to paginate?
- [ ] Do error responses follow the three-part template (what, expected, example)?
- [ ] Is every panic/exception caught inside the handler?
