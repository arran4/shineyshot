# Contributor Guidelines

After making any code changes, ensure the following commands run without errors before committing:

```
# Format the code
go fmt ./...

# Vet the code for issues
go vet ./...

# Run all tests
go test ./...
```

# Visual Verification

To generate screenshots of the UI in specific configurations for verification purposes, use the `test verification` command. This is useful for automated visual testing or generating documentation screenshots.

Usage:
```
shineyshot test verification -input config.json -output screenshot.png
```

To generate screenshots using a specific theme:

```
shineyshot -theme dark test verification -input config.json -output screenshot_dark.png
```

Supported themes: `default`, `dark`, `high_contrast`, `hotdog`.

The configuration file is a JSON object defining the application state. Example:

```json
{
  "width": 800,
  "height": 600,
  "current_tab": 0,
  "tool": 2,
  "color_idx": 2,
  "number_idx": 0,
  "cropping": false,
  "crop_rect": [0, 0, 0, 0],
  "crop_start": [0, 0],
  "text_input_active": false,
  "text_input": "",
  "text_pos": [0, 0],
  "message": "",
  "annotation_enabled": true,
  "version_label": "v1.2.3",
  "tabs": [
    {
      "title": "Tab 1",
      "offset": [0, 0],
      "zoom": 1.0,
      "next_number": 1,
      "width_idx": 2,
      "shadow_applied": false,
      "image_color": [255, 255, 255, 255],
      "image_size": [600, 400]
    }
  ]
}
```
