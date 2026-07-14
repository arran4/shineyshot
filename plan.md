1. **Architectural Additions (`internal/skill/`)**
   - Create abstractions: `Target`, `Source`, `Installer`, `Metadata`.
   - Implement `NeutralTarget` for standard `.agents/skills` directories.
   - Implement `LocalSource` for local paths and `GitHubSource` for `owner/repo`.
   - Implement sidecar metadata logic to save source/provenance state (`metadata.json`).
   - Implement installation, update, remove, and list functionality with atomic directory creation.
2. **CLI Additions (`cmd/shineyshot/`)**
   - Add `skill.go` parsing the `skill` subcommand.
   - Implement handlers: `install`, `update`, `remove`, `list`, `inspect`.
   - Register the `skill` subcommand in `main.go`.
   - Update `help.go` and add text templates in `cmd/shineyshot/templates/`.
3. **Official Skill Content (`skills/shineyshot/`)**
   - Create a thorough `SKILL.md` detailing operational guidance, traps, UI modes vs CLI modes, and non-interactive usage.
4. **Testing**
   - Add unit tests for skill components (`internal/skill/installer_test.go`, etc.).
   - Ensure CLI parsing tests cover skill subcommands.
5. **Pre Commit Steps**
   - Complete pre-commit steps to ensure proper testing, verification, review, and reflection are done.
6. **Self-Hosting Verification**
   - Run the compiled binary to install its own official skill.
