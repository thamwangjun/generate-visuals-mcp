# Phase 1: Core Server + Image Generation Tool - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-05-27
**Phase:** 1-Core Server + Image Generation Tool
**Areas discussed:** Project layout, Tool description text, .env file loading, Error response content

---

## Project Layout

| Option | Description | Selected |
|--------|-------------|----------|
| Flat (main.go only) | Everything in package main. Will need to split in Phase 2. | |
| Two packages (main + tools/) | tools/ package alongside main.go. Phase 2 adds auth/ alongside. | |
| Full structure upfront | main.go + internal/tools/ + internal/config/ + internal/auth/ scaffold | ✓ |

**User's choice:** Full structure upfront

**Follow-up — package location:**

| Option | Description | Selected |
|--------|-------------|----------|
| internal/ | internal/tools/, internal/config/, internal/auth/. Prevents accidental external import. | ✓ |
| Top-level | tools/, config/, auth/ at project root. | |

**User's choice:** internal/

**Notes:** Auth package scaffolded but empty in Phase 1 so Phase 2 is a straight diff rather than a restructure.

---

## Tool Description Text

| Option | Description | Selected |
|--------|-------------|----------|
| Input/output focused | Lead with what it takes/returns, when-to-use, when-not-to-use. | |
| Use-case focused | Lead with agent workflow and examples. | |
| I'll provide the description | User has specific wording in mind. | ✓ |

**User's choice:** "Create a Go template with a TODO placeholder that you will import for tool desc."

**Follow-up — placeholder storage:**

| Option | Description | Selected |
|--------|-------------|----------|
| Go const in internal/tools/ | `const generateVisualsDescription = ...` with TODO comment. | ✓ |
| Separate embedded file | description.txt via go:embed. | |

**User's choice:** Go const in internal/tools/

**Notes:** Description content is deferred — the const placeholder lets it be filled in without touching handler logic.

---

## .env File Loading

| Option | Description | Selected |
|--------|-------------|----------|
| godotenv library | github.com/joho/godotenv. Handles edge cases, 1 dep. | ✓ |
| Manual parser | ~15 lines, zero deps. Brittle for quoted values. | |
| You decide | Claude picks idiomatic approach. | |

**User's choice:** godotenv library

**Follow-up — missing .env behavior:**

| Option | Description | Selected |
|--------|-------------|----------|
| Silent skip | Continue if .env absent. Fail only on missing required vars. | ✓ |
| Warning log | Log a warning, then continue. | |
| Hard fail | Exit if .env missing. | |

**User's choice:** Silent skip

---

## Error Response Content

| Option | Description | Selected |
|--------|-------------|----------|
| Actionable with retry hint | 3-part template: what failed, why, recovery suggestion. | ✓ |
| Pass-through Gemini error | Raw SDK error message. | |
| Generic message only | 'Image generation failed. Try again.' | |

**User's choice:** Actionable with retry hint

**Follow-up — empty response handling:**

| Option | Description | Selected |
|--------|-------------|----------|
| Treat as error with isError:true | Structured error with content-policy hint and prompt rephrase suggestion. | ✓ |
| Return empty ImageContent | Undefined client behaviour. | |
| Panic / hard fail | Not recommended per reference doc. | |

**User's choice:** Treat as error with isError:true

---

## Claude's Discretion

- Tool annotations (`openWorldHint`, `readOnlyHint`, `destructiveHint`, `idempotentHint`) — set per reference doc decision table, not asked.
- Server name/version (`"generate-visuals-mcp"`, `"1.0.0"`) — inferred from project name.
- `image_prompt` parameter name — fixed by REQUIREMENTS.md TOOL-02, not discussed.

## Deferred Ideas

None — discussion stayed within phase scope.
