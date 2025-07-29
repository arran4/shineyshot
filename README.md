# ShineyShot

ShineyShot is a minimal screenshot and annotation tool built with
[golang.org/x/exp/shiny](https://pkg.go.dev/golang.org/x/exp/shiny) for the UI.
Screenshots are captured directly via the
[XDG Desktop Portal](https://flatpak.github.io/xdg-desktop-portal/) screenshot
API on Wayland compositors.
It now includes a simple tabbed interface similar to classic paint programs.

## Features

- Capture a screenshot using the portal if no existing image is provided.
- Draw annotations with the mouse.
- Press **s** to save the annotated image.
- Press **n** to capture a new screenshot in a new tab.
- Press **q** to quit.

## Requirements

- Go 1.20+ (tested with Go 1.24).
- A running XDG desktop portal implementation for screenshot support.

## Usage

```
# Capture the entire screen and annotate
shineyshot

# Annotate an existing image
shineyshot -screenshot input.png -output result.png
```

The output file defaults to `annotated.png` in the current directory.

