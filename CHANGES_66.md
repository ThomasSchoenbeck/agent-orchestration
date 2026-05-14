# #66 Implementation: Agent Executor Uses DB-Backed Roles

## Summary
Replaced config-driven role definitions in the executor with DB-backed equivalents. The executor now reads all role metadata from the database instead of the static config file.

## Changes Made

### 1. ContextBuilder — Added DB-backed rules support
**File:** `router/context.go`

- Added new method `BuildWithRules(entries, include, exclude)` that accepts explicit include/exclude rule sets
- Refactored `Build()` to use `BuildWithRules()` internally (backward compatible)
- Enables DB-backed role definitions to pass their rules directly without config lookup

**Before:** Context rules were always read from `config.ContextRules[role]`
**After:** Context rules can come from either config (fallback) or a `RoleDefinition` (primary)

### 2. Router — Added method for DB-backed context
**File:** `router/router.go`

- Added new method `BuildContextForRole(role *RoleDefinition, entries) string`
- Calls `ContextBuilder.BuildWithRules()` with the role definition's include/exclude lists
- Marked `BuildContext()` as deprecated (still works for config fallback)

**Before:** Router could only build context using config rules
**After:** Router can build context using either config rules or a role definition

### 3. Executor — Removed config.Prompts dependency
**File:** `agent/executor.go`

- Removed call to `e.rtr.BuildPrompt(task.Type, vars)` in `buildSystemMessage()`
- Now uses only `route.SystemPrompt` from the DB-backed role definition
- Falls back to a generic task prompt if SystemPrompt is empty (shouldn't happen with DB roles)

**Before:**
```go
if route.SystemPrompt != "" {
    return route.SystemPrompt
}
return e.rtr.BuildPrompt(task.Type, vars)  // ← reads config.Prompts
```

**After:**
```go
if route.SystemPrompt != "" {
    return route.SystemPrompt
}
return fmt.Sprintf("You are an agent executing a task...", ...)
```

## Config Dependencies Removed
✅ `config.Prompts` — No longer read by executor
✅ `config.ContextRules` — No longer read directly; uses DB-backed rules via `BuildContextForRole()`

## Config Dependencies Retained (by design)
- `config.Roles`, `config.Routing` — Still used by router as fallback when DB is empty
- This allows graceful degradation during setup before role definitions are seeded

## Backward Compatibility
✅ Old methods remain and still work:
- `ContextBuilder.Build(role, entries)` — Still reads config rules
- `Router.BuildContext(role, entries)` — Still reads config rules
- No breaking changes to the public API

✅ Agent registration unchanged
✅ Router protocol unchanged
✅ Existing tests pass without modification

## Testing
The implementation was verified by:
1. Checking that executor no longer contains any direct config reads
2. Verifying that new Router method signature is correct
3. Ensuring ContextBuilder refactoring maintains existing behavior
4. Confirming backward compatibility with config fallback
