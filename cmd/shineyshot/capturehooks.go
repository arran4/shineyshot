package main

import "github.com/example/shineyshot/internal/capture"

var (
	captureScreenshotFn = capture.Screenshot
	captureWindowFn     = capture.Window
	captureRegionFn     = capture.Region
	captureRegionRectFn = capture.RegionRect
)
