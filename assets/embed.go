package assets

import (
	"bytes"
	"embed"
	"fmt"
	"image"
	"image/png"
	"io/fs"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// Embedded icon assets for ShineyShot.
//
//go:embed icons/*.png icons/*.svg
var embeddedIcons embed.FS

var (
	loadIconsOnce sync.Once
	loadIconsErr  error

	pngImages = map[int]image.Image{}
	pngData   = map[int][]byte{}
	svgData   []byte
)

func loadIcons() {
	entries, err := fs.ReadDir(embeddedIcons, "icons")
	if err != nil {
		loadIconsErr = err
		return
	}
	for _, entry := range entries {
		name := entry.Name()
		data, err := embeddedIcons.ReadFile(filepath.Join("icons", name))
		if err != nil {
			loadIconsErr = err
			return
		}
		switch {
		case strings.HasSuffix(name, ".png"):
			base := strings.TrimSuffix(name, ".png")
			idx := strings.LastIndex(base, "-")
			if idx == -1 || idx == len(base)-1 {
				continue
			}
			sizeVal := base[idx+1:]
			size, err := strconv.Atoi(sizeVal)
			if err != nil {
				continue
			}
			img, err := png.Decode(bytes.NewReader(data))
			if err != nil {
				loadIconsErr = err
				return
			}
			pngImages[size] = img
			buf := make([]byte, len(data))
			copy(buf, data)
			pngData[size] = buf
		case strings.HasSuffix(name, ".svg"):
			svgData = append([]byte(nil), data...)
		}
	}
}

func ensureIcons() error {
	loadIconsOnce.Do(loadIcons)
	return loadIconsErr
}

// IconImage returns the decoded image for an embedded PNG icon of the requested size.
func IconImage(size int) (image.Image, error) {
	if err := ensureIcons(); err != nil {
		return nil, err
	}
	img, ok := pngImages[size]
	if !ok {
		return nil, fmt.Errorf("icon %dpx not embedded", size)
	}
	return img, nil
}

// IconPNG returns a copy of the raw PNG bytes for the requested icon size.
func IconPNG(size int) ([]byte, error) {
	if err := ensureIcons(); err != nil {
		return nil, err
	}
	data, ok := pngData[size]
	if !ok {
		return nil, fmt.Errorf("icon %dpx not embedded", size)
	}
	out := make([]byte, len(data))
	copy(out, data)
	return out, nil
}

// IconSizes lists the icon sizes embedded in the binary.
func IconSizes() []int {
	if err := ensureIcons(); err != nil {
		return nil
	}
	sizes := make([]int, 0, len(pngImages))
	for size := range pngImages {
		sizes = append(sizes, size)
	}
	sort.Ints(sizes)
	return sizes
}

// IconSVG returns the SVG icon bytes if present.
func IconSVG() ([]byte, error) {
	if err := ensureIcons(); err != nil {
		return nil, err
	}
	if len(svgData) == 0 {
		return nil, fmt.Errorf("svg icon not embedded")
	}
	out := make([]byte, len(svgData))
	copy(out, svgData)
	return out, nil
}
