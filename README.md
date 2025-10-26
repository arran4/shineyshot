<p align="center">
  <img src="assets/icons/shineyshot-128.png" alt="ShineyShot icon" width="128" />
</p>

# ShineyShot

ShineyShot brings capture and annotation tools together across four complementary modes so workflows can move from a quick markup to a fully scripted pipeline without switching apps. Updated screencaps for every workflow will land soon.

## UI Mode

Launch the graphical editor from any environment and control how it starts up with command-line flags.

```bash
# Open the editor directly on an existing image
shineyshot annotate -file snapshot.png open

# Capture a window by selector and jump straight into annotation
shineyshot annotate capture window "Settings Panel"

# Start in capture region mode with a preset rectangle
shineyshot annotate capture region 0,0,1440,900
```

### Screenshot

_Annotated workflow screenshot coming soon._

## Modes at a Glance

- **UI mode** keeps the graphical editor front and centre for drag-and-drop annotation, layering, and exporting.
- **CLI file mode** automates repeatable capture and markup tasks without leaving the terminal.
- **CLI background mode** keeps a session alive so other commands—or other people—can reuse the same permissions.
- **Interactive mode** gives you a text-driven shell with history and inline help.

## CLI File Mode

Group repeated operations on a file behind the `file` subcommand. The file path is supplied once and passed to nested commands unless you override it.

```bash
sh-5.3$ shineyshot file -file snapshot.png capture screen
saved /home/user/Pictures/snapshot.png
sh-5.3$ shineyshot file -file snapshot.png draw line 10 10 200 120
saved /home/user/Pictures/snapshot.png
sh-5.3$ shineyshot file -file snapshot.png preview
```

Nested commands can still set `-file` or `-output` to redirect work elsewhere:

```bash
sh-5.3$ shineyshot file -file snapshot.png draw -output annotated.png arrow 0 0 320 240
saved /home/user/Pictures/annotated.png
```

### Capture screenshots on Linux

ShineyShot talks to the desktop portal and prints Linux-friendly status messages describing where the image is saved. Pick from three capture modes:

```bash
# Capture the entire display (default)
sh-5.3$ shineyshot file -file screenshot.png capture screen 0
saved /home/user/Pictures/screenshot.png

# Capture the currently active portal window
sh-5.3$ shineyshot file -file screenshot.png capture window firefox

# Capture a specific rectangle (x0,y0,x1,y1)
sh-5.3$ shineyshot file -file screenshot.png capture region 0,0,640,480
```

Provide an optional selector argument—or `-select` for scripts—to target a specific display or window. Supply regions with the
`-rect` flag or trailing `x0,y0,x1,y1` coordinates.

Pass `--stdout` to write the PNG bytes to stdout instead of creating a file.

### Draw quick markup

Apply lightweight annotations to an existing image. Lines and arrows expand the canvas as needed so their endpoints stay visible.

Shapes accept the following coordinate formats. Each row pairs the argument list with a complete command you can paste into a script or terminal session:

| Shape  | Arguments         | Example command |
| ------ | ----------------- | --------------- |
| line   | `x0 y0 x1 y1`     | `shineyshot file -file input.png draw line 10 10 200 120` |
| arrow  | `x0 y0 x1 y1`     | `shineyshot file -file input.png draw -color green arrow 10 10 200 160` |
| rect   | `x0 y0 x1 y1`     | `shineyshot file -file input.png draw rect 10 10 220 160` |
| circle | `cx cy radius`    | `shineyshot file -file input.png draw circle 120 120 30` |
| number | `x y value`       | `shineyshot file -file input.png draw number 40 80 1` |
| text   | `x y "string"`   | `shineyshot file -file input.png draw text 60 120 "Review"` |
| mask   | `x0 y0 x1 y1`     | `shineyshot file -file input.png draw mask 20 20 180 140` |

### CLI automation example

Bundle capture and annotation into a single script when building CI jobs or local helpers:

```bash
#!/usr/bin/env bash
set -euo pipefail

output_dir="${1:-./runs}"
mkdir -p "$output_dir"

target="$output_dir/$(date +%F)-dashboard.png"

shineyshot file -file "$target" capture window goland
shineyshot file -file "$target" draw text 40 60 "Build: ${CI_PIPELINE_ID:-local}"
shineyshot file -file "$target" draw arrow 120 120 320 180
```

## CLI Background Mode

Run ShineyShot as a background service and communicate via UNIX sockets. The daemon runs within the current user session so scripts can reuse capture permissions without additional prompts.

```bash
# Start a named background session (socket stored in $XDG_RUNTIME_DIR/shineyshot or ~/.shineyshot/sockets)
sh-5.3$ shineyshot background start MySession
started background session MySession at /run/user/1000/shineyshot/MySession.sock

# List all active sessions
sh-5.3$ shineyshot background list
available sockets:
  MySession

# Attach to a running session for live interaction
sh-5.3$ shineyshot background attach MySession
> arrow 0 0 320 240
no image loaded
> ^D

# Run a single command within the session
sh-5.3$ shineyshot background run MySession capture screen
captured screen current display
sh-5.3$ shineyshot background attach MySession
> arrow 0 0 320 240
arrow drawn
> ^D

# Stop and clean up when finished
sh-5.3$ shineyshot background stop MySession
stop requested for MySession
```

Add `background serve` when embedding ShineyShot into another long-lived process. Store helpers alongside other dotfiles utilities; for example, `~/.local/bin/shineyshot-window` can wrap `shineyshot background run MySession capture window "$1"` so scripts capture consistent evidence before processing.

## Interactive Mode

Use the text-driven shell for command discovery, history, and inline execution:

```bash
sh-5.3$ shineyshot interactive
Interactive mode. Type 'help' for commands.
> help
Commands:
  capture screen [DISPLAY]   capture full screen; use 'screens' to list displays
  capture window SELECTOR    capture window by selector; use 'windows' to list options
  capture region SCREEN X Y WIDTH HEIGHT   capture region on a screen; 'screens' lists displays
  arrow x0 y0 x1 y1          draw arrow with current stroke
  line x0 y0 x1 y1           draw line with current stroke
  rect x0 y0 x1 y1           draw rectangle with current stroke
  circle x y r               draw circle with current stroke
  crop x0 y0 x1 y1           crop image to rectangle
  color [value|list]         set or list palette colors
  colors                     list palette colors
  width [value|list]         set or list stroke widths
  widths                     list stroke widths
  show                       open synced annotation window
  preview                    open copy in separate window
  save FILE                  save image to FILE
  savetmp                    save to /tmp with a unique filename
  savepictures               save to your Pictures directory
  savehome                   save to your home directory
  copy                       copy image to clipboard
  windows                    list available windows and selectors
  screens                    list available screens/displays
  copyname                   copy last saved filename
  background start [NAME] [DIR]   launch a background socket session
  background stop [NAME] [DIR]    stop a background socket session
  background list [DIR]           list background sessions
  background clean [DIR]          remove dead background sockets
  background run [NAME] COMMAND [ARGS...]   run a socket command (e.g., 'background run capture screen')
  quit                       exit interactive mode

Window selectors:
  index:<n>        window list index (see 'windows')
  id:<hex|dec>     X11 window id
  pid:<pid>        process id that owns the window
  exec:<name>      executable name substring
  class:<name>     X11 WM_CLASS substring
  title:<text>     window title substring (useful for literal words like 'list')
  <text>           fallback substring match on title/executable/class
```

From inside the shell, run commands such as `capture window` or `draw rect 10 10 200 180`. You can also pre-seed commands when launching:

```bash
sh-5.3$ shineyshot interactive -e "capture screen" -e "rect 10 10 200 200"
captured screen current display
rectangle drawn
```

## License

ShineyShot is licensed under the GNU Affero General Public License v3.0. See [LICENSE](LICENSE) for details.
