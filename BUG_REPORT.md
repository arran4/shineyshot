# Migration to `go-subcommand` (gosubc) Blockers

Attempting to migrate `shineyshot` CLI to `github.com/arran4/go-subcommand` (v0.0.11/v0.0.12) identified several critical blockers that prevent maintaining the existing CLI interface and functionality without significant regressions or rewrites.

## 1. Strict Flag Validation (No Pass-Through)

The core issue is that `gosubc` generates a `Execute` method for commands that strictly validates all arguments starting with `-`.

```go
// Generated code example
if strings.HasPrefix(arg, "-") {
    // ...
    default:
        return fmt.Errorf("unknown flag: %s", name)
}
```

This makes it impossible to define a "passthrough" command like `Annotate(args ...string)` where flags (e.g., `-file`, `-shadow`) are passed raw to the underlying implementation. The generated code intercepts and rejects them as "unknown flags".

To support the existing interface:
*   We would have to redefine *every* flag for every subcommand in the `gosubc` comments.
*   This would duplicate the flag definitions currently held in `internal/cli` (e.g., `annotateCmd` struct).
*   The implementation logic would need to be rewritten to accept parsed values instead of `[]string`, breaking the existing encapsulation and requiring significant changes to the logic (risk of functional loss).

## 2. Global Initialization Hook

The existing application initializes a global state (`Root`) with configuration and themes derived from global flags (`-theme`, `-notify-...`).

`gosubc` v0.0.12 supports root flags, but the generated `Execute` method only calls the root command function if *no subcommand* is matched.

```go
// Generated code logic
if len(remainingArgs) > 0 {
    // dispatch to subcommand
    return cmd.Execute(...)
}
Root(...) // Only called if no subcommand
```

This means we cannot easily hook global initialization logic *before* a subcommand runs, unless we manually patch the generated `root.go` file. Manual patching is fragile because `go generate` will overwrite the changes.

## 3. Custom Help Templates

`shineyshot` uses detailed, custom help templates (e.g., flag groups). `gosubc` enforces its own generated help format (or requires overriding usage methods via manual code). While we can bypass this by calling our own help functions, the rigid structure of the generated code makes this integration messy and reliant on manual patches.

## 4. Go Version Requirement

`gosubc` v0.0.12 declares a requirement for Go `1.25.3+` in its `go.mod`. This causes `go get` to update the project's `go.mod` to an invalid/future Go version, potentially breaking build environments.

## Conclusion

Migrating to `gosubc` while preserving the exact CLI behavior is currently not feasible without upstream changes to `gosubc` (e.g., a "PassThrough" option for unknown flags) or accepting a complete rewrite of the argument parsing logic that duplicates definitions and abandons the existing `flag.FlagSet` based implementations.
