# ShineyShot

ShineyShot brings capture and annotation tools together across four complementary modes so workflows can move from a quick markup to a fully scripted pipeline without switching apps. Updated screencaps for every workflow will land soon.

## UI Mode

Launch the graphical editor from any environment and control how it starts up with command-line flags.

```bash
# Open the editor directly on an existing image
shineyshot annotate open-file -file snapshot.png -output annotated.png

# Capture a window and pre-fill the title for the exported asset
shineyshot annotate capture-window --name "Settings Panel" --output annotated-settings.png

# Start in capture-region mode with a preset aspect ratio and export folder
shineyshot annotate capture-region --region 0,0,1440,900 --output ./exports/tutorial-step.png
```

### Capture-to-export checklist

1. **Start the editor** – run `shineyshot annotate` to open the canvas.
2. **Capture** – pick **Capture screen**, **Capture window**, or **Capture region**. Region capture provides crosshair guides and zoomed previews for precise selection.
3. **Annotate** – add arrows, boxes, text callouts, and blur masks. The layer sidebar tracks visibility, order, and naming.
4. **Adjust** – constrain angles with <kbd>Shift</kbd>, apply saved colour presets, or toggle grid snapping for alignment.
5. **Export** – save to PNG, copy to clipboard, or set a default path with `shineyshot annotate --output annotated.png`.

### Repeatable GUI session

Store launcher flags in a shell script so the same capture layout is always available:

```bash
#!/usr/bin/env bash
# ~/.local/bin/shineyshot-daily
set -euo pipefail
export SHINEYSHOT_EXPORT_DIR="$HOME/Pictures/shineyshot"

shineyshot annotate capture-window \
  --name "Daily status" \
  --output "${SHINEYSHOT_EXPORT_DIR}/$(date +%F)-status.png"
```

Make the script executable and bind it to a desktop shortcut or window manager hotkey.

---

## Modes at a Glance

- **UI mode** keeps the graphical editor front and centre for drag-and-drop annotation, layering, and exporting.
- **CLI file mode** automates repeatable capture and markup tasks without leaving the terminal.
- **CLI background mode** keeps a session alive so other commands—or other people—can reuse the same permissions.
- **Interactive mode** gives you a text-driven shell with history and inline help.

---

## CLI File Mode

Group repeated operations on a file behind the `file` subcommand. The file path is supplied once and passed to nested commands unless you override it.

```bash
shineyshot file -file snapshot.png snapshot --mode screen
shineyshot file -file snapshot.png draw line 10 10 200 120
shineyshot file -file snapshot.png preview
```

Nested commands can still set `-file` or `-output` to redirect work elsewhere:

```bash
shineyshot file -file snapshot.png draw -output annotated.png arrow 0 0 320 240
```

### Capture screenshots on Linux

ShineyShot talks to the XDG desktop portal and prints Linux-friendly status messages describing where the image is saved. Pick from three capture modes:

```bash
# Capture the entire display (default)
shineyshot snapshot --mode screen --display HDMI-A-1

# Capture the currently active portal window
shineyshot snapshot --mode window --window firefox

# Capture a specific rectangle (x0,y0,x1,y1)
shineyshot snapshot --mode region --region 0,0,640,480
```

Pass `--stdout` to write the PNG bytes to stdout instead of creating a file.

### Draw quick markup

Apply lightweight annotations to an existing image. Lines and arrows expand the canvas as needed so their endpoints stay visible.

Shapes accept the following coordinate formats. Each row pairs the argument list with a complete command you can paste into a script or terminal session:

| Shape  | Arguments         | Example command |
| ------ | ----------------- | --------------- |
| line   | `x0 y0 x1 y1`     | `shineyshot draw -file input.png line 10 10 200 120` |
| arrow  | `x0 y0 x1 y1`     | `shineyshot draw -file input.png arrow -color green 10 10 200 160` |
| rect   | `x0 y0 x1 y1`     | `shineyshot draw -file input.png rect 10 10 220 160` |
| circle | `cx cy radius`    | `shineyshot draw -file input.png circle 120 120 30` |
| number | `x y value`       | `shineyshot draw -file input.png number 40 80 1` |
| text   | `x y "string"`   | `shineyshot draw -file input.png text 60 120 "Review"` |
| mask   | `x0 y0 x1 y1`     | `shineyshot draw -file input.png mask 20 20 180 140` |

### CLI automation example

Bundle capture and annotation into a single script when building CI jobs or local helpers:

```bash
#!/usr/bin/env bash
set -euo pipefail

output_dir="${1:-./runs}"
mkdir -p "$output_dir"

target="$output_dir/$(date +%F)-dashboard.png"

shineyshot snapshot --mode window --window nightly-dashboard --output "$target"
shineyshot draw -file "$target" text 40 60 "Build: ${CI_PIPELINE_ID:-local}" \
  arrow 120 120 320 180
```

---

## CLI Background Mode

Run ShineyShot as a background service and communicate via UNIX sockets. The daemon runs within the current user session so scripts can reuse capture permissions without additional prompts.

```bash
# Start a named background session (socket stored in $XDG_RUNTIME_DIR/shineyshot or ~/.shineyshot/sockets)
shineyshot background start team-room

# List all active sessions
shineyshot background list

# Attach to a running session for live interaction
shineyshot background attach team-room

# Run a single command within the session
shineyshot background run team-room snapshot --mode screen

# Stop and clean up when finished
shineyshot background stop team-room
```

Add `background serve` when embedding ShineyShot into another long-lived process. Store helpers alongside other dotfiles utilities; for example, `~/.local/bin/shineyshot-window` can wrap `shineyshot background run default snapshot --mode window --window "$1"` so scripts capture consistent evidence before processing.

---

## Interactive Mode

Use the text-driven shell for command discovery, history, and inline execution:

```bash
shineyshot interactive
```

From inside the shell, run commands such as `snapshot --mode window` or `draw rect 10 10 200 180`. You can also pre-seed commands when launching:

```bash
shineyshot interactive -e "snapshot --mode screen" -e "draw rect 10 10 200 200"
```

To drive the shell against a background session, supply the session name and optional socket directory:

```bash
shineyshot interactive -name team-room -dir /run/user/1000/shineyshot
```

### Interactive scripting example

Capture a repeatable sequence, export it, and reuse it elsewhere:

```bash
shineyshot interactive <<'EOF'
snapshot --mode screen
draw rect 20 20 420 280
:export ./scripts/capture-flow.sh
EOF
```

---

## License

ShineyShot is licensed under the GNU Affero General Public License v3.0. See [LICENSE](LICENSE) for details.
