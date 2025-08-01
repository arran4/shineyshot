# ShineyShot

ShineyShot provides a simple command line interface for capturing and annotating screenshots.

## Examples

Capture and annotate a screenshot from the screen:

```
shineyshot annotate capture-screen
```

Annotate an existing image file:

```
shineyshot annotate open-file -file image.png -output annotated.png
```

Preview an image in a window:

```
shineyshot preview -file annotated.png
```

Capture a screenshot directly to a file:

```
shineyshot snapshot -output screenshot.png
```

Draw on an existing image:

```
shineyshot draw line -file input.png -output output.png 0 0 100 100
```

Other shapes such as arrows or numbered markers will use similar syntax:

```
shineyshot draw arrow -file input.png -output output.png 0 0 100 100
shineyshot draw number -file input.png -output output.png 40 40 3
```

Currently only the `line` shape is implemented. Support for additional shapes
may be added in the future.

The `snapshot` command will gain options for capturing windows and regions in future versions.
