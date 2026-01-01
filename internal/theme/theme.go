package theme

import (
	"image/color"
)

// Theme defines the color palette for the application UI.
type Theme struct {
	Name string

	// General
	Background color.RGBA // Main window background (behind tabs/canvas)
	Foreground color.RGBA // Main text color

	// Toolbar & Tabs
	ToolbarBackground color.RGBA
	TabBackground     color.RGBA // Inactive tab background
	TabActive         color.RGBA // Active tab background
	TabHover          color.RGBA
	TabText           color.RGBA
	TabTextActive     color.RGBA
	TabTextHover      color.RGBA

	// Tool Buttons
	ButtonBackground      color.RGBA
	ButtonBackgroundHover color.RGBA
	ButtonBackgroundPress color.RGBA
	ButtonText            color.RGBA
	ButtonTextHover       color.RGBA
	ButtonTextPress       color.RGBA
	ButtonBorder          color.RGBA

	// Canvas
	CheckerLight color.RGBA
	CheckerDark  color.RGBA
}

// Default returns the hardcoded default light theme (fallback).
func Default() *Theme {
	return &Theme{
		Name:                  "Default",
		Background:            color.RGBA{220, 220, 220, 255},
		Foreground:            color.RGBA{0, 0, 0, 255},
		ToolbarBackground:     color.RGBA{220, 220, 220, 255},
		TabBackground:         color.RGBA{220, 220, 220, 255},
		TabActive:             color.RGBA{200, 200, 200, 255},
		TabHover:              color.RGBA{210, 210, 210, 255},
		TabText:               color.RGBA{0, 0, 0, 255},
		TabTextActive:         color.RGBA{0, 0, 0, 255},
		TabTextHover:          color.RGBA{0, 0, 0, 255},
		ButtonBackground:      color.RGBA{200, 200, 200, 255},
		ButtonBackgroundHover: color.RGBA{180, 180, 180, 255},
		ButtonBackgroundPress: color.RGBA{150, 150, 150, 255},
		ButtonText:            color.RGBA{0, 0, 0, 255},
		ButtonTextHover:       color.RGBA{0, 0, 0, 255},
		ButtonTextPress:       color.RGBA{0, 0, 0, 255},
		ButtonBorder:          color.RGBA{0, 0, 0, 255},
		CheckerLight:          color.RGBA{220, 220, 220, 255},
		CheckerDark:           color.RGBA{192, 192, 192, 255},
	}
}
