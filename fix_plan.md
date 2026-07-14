1. **Fix Arbitrary File Deletion / Path Traversal via `skillName`:**
   - In `internal/skill/installer.go` and `cmd/shineyshot/skill.go`, validate `skillName` to ensure it only contains safe characters (alphanumeric, dashes, underscores) and does not contain `.` or `/`.

2. **Fix Malicious Symlinks Vulnerability:**
   - In `internal/skill/installer.go` within `Install`, check `d.Type()` for `os.ModeSymlink`. Disallow or skip symlinks completely to avoid information disclosure.

3. **Fix Local Modification Detection:**
   - When a skill is updated, we must compare the local directory hash/state with the remote state, or at least decline updates if the file has been modified locally. Let's add a simple check in `Update` that reads the `SKILL.md` content digest or modification time and warns the user unless `-force` is used.

4. **Fix Duplicate Overwrites in Install:**
   - In `internal/skill/installer.go`'s `Install` method, if the destination `skillDest` already exists, return an error (e.g., "skill already exists") unless it's explicitly an update flow. Add an `isUpdate` parameter.

5. **Fix Go:Embedded Official Skill:**
   - Move `skills/shineyshot/SKILL.md` into an embeddable package and create `skills/shineyshot/embed.go` with `//go:embed SKILL.md`.

6. **Refactor GitHub Fetching (Optional but good):**
   - Instead of `git clone`, use `net/http` (the app's existing client behavior) to fetch the tarball from GitHub.
