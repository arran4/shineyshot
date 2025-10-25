# ShineyShot

ShineyShot provides a simple command line interface for capturing and annotating screenshots.

## Examples

### Work with an existing file

Group repeated operations on a file behind the `file` subcommand. The file path
is supplied once up front and is passed through to the nested command unless you
explicitly override it later.

```
shineyshot file -file snapshot.png snapshot --mode screen
shineyshot file -file snapshot.png draw line 10 10 200 120
shineyshot file -file snapshot.png preview
```

The nested command can still override `-file` or `-output` if you want to write
to a different location:

```
shineyshot file -file snapshot.png draw -output annotated.png arrow 0 0 320 240
```

### Capture screenshots on Linux

ShineyShot talks to the XDG desktop portal and prints Linux-friendly status
messages describing where the image is saved. Pick from three capture modes:

```
# Capture the entire display (default)
shineyshot snapshot --mode screen --display HDMI-A-1

# Capture the currently active portal window
shineyshot snapshot --mode window --window firefox

# Capture a specific rectangle (x0,y0,x1,y1)
shineyshot snapshot --mode region --region 0,0,640,480
```

Pass `--stdout` to write the PNG bytes to stdout instead of creating a file.

### Draw quick markup

Apply lightweight annotations to an existing image. Lines and arrows expand the
canvas as needed so their endpoints stay visible.

```
shineyshot draw -file input.png rect 10 10 220 160
shineyshot draw -file input.png circle 120 120 30
shineyshot draw -file input.png arrow -color green 10 10 200 160
```

Shapes accept the following coordinate formats:

| Shape  | Arguments                   |
| ------ | --------------------------- |
| line   | `x0 y0 x1 y1`               |
| arrow  | `x0 y0 x1 y1`               |
| rect   | `x0 y0 x1 y1`               |
| circle | `cx cy radius`              |
| number | `x y value`                 |
| text   | `x y "string"`             |
| mask   | `x0 y0 x1 y1`               |

### Launch the annotation UI

Open the interactive editor using the `annotate` command. You can capture a new
screenshot or open an existing file.

```
shineyshot annotate capture-screen
shineyshot annotate capture-window
shineyshot annotate capture-region
shineyshot annotate open-file -file snapshot.png -output annotated.png
```

