# ShineyShot Skill

Welcome to the official ShineyShot agent skill. This document provides operational guidance for AI coding agents to effectively and correctly use ShineyShot.

## Overview

ShineyShot is a screen capture and annotation CLI application. It provides multiple ways to capture and annotate images.

### Understanding Modes
- **UI Mode (`annotate`)**: Launches an interactive GUI for human users. Use this mode when generating graphical configurations or when a user requests "open the UI". It blocks until the window is closed.
- **CLI File Mode (`file`)**: A non-interactive pipeline to automate capture and drawing on a single file. Essential for shell scripts and automated testing without launching the GUI.
- **Background Mode (`background`)**: A daemon service allowing multiple commands to run under the same OS capture permission session.
- **Snapshot Mode (`snapshot`)**: Fast, one-off captures of a screen, window, or region.

## Common Agent Traps

### 1. `snapshot` vs `file capture`
When automating screenshot pipelines, be mindful of whether you need to apply annotations immediately.
- Use `shineyshot snapshot [target] -output file.png` for quick image grabbing.
- Use `shineyshot file -file test.png capture [target]` if you immediately intend to run `shineyshot file -file test.png draw [shape]`.

### 2. Missing Context or Interactive Blocking
If an interactive UI mode opens (e.g., just running `shineyshot annotate`), the process will block waiting for user input. For any automated agent task, always prefer the `snapshot` or `file` commands which are non-blocking.

### 3. Coordinate Spaces
When using `draw` commands (e.g. `draw rect x0 y0 x1 y1`), the coordinates are relative to the *image itself*, not the global screen coordinates. However, `capture region x0,y0,x1,y1` uses *global screen coordinates*. Do not mix them up.

### 4. Background Server State
If you attempt to use `background run <session>`, verify the session is started (`background start <session>`) first. If it's dead, run `background clean`.

## Configuration Options

ShineyShot resolves settings in the following order:
1. CLI Flags (`-theme dark`)
2. Environment Variables (`SHINEYSHOT_THEME=dark`)
3. RC Config (`shineyshot config print`)
4. Internal defaults

To quickly view settings, run `shineyshot config print`.

## Safe Mutations

Use dry-runs and verify files before and after modifications. There is no explicit `-dry-run` flag for mutations, but you can utilize `-output /tmp/test.png` before overwriting primary assets.

## Examples

**Example 1: Capture active window and draw an arrow (Non-interactive Pipeline)**
```bash
# Capture the window
shineyshot file -file /tmp/evidence.png capture window
# Draw a red arrow
shineyshot file -file /tmp/evidence.png draw -color red arrow 10 10 200 120
```

**Example 2: Verify existing configuration**
```bash
shineyshot config print
```

**Example 3: Run via background to reuse permissions**
```bash
shineyshot background start ci-session
shineyshot background run ci-session capture screen
shineyshot background stop ci-session
```
