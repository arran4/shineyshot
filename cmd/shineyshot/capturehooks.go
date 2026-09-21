package main

import "github.com/arran4/shineyshot/internal/capture"

var (
	captureScreenshotFn = capture.Screenshot
	captureWindowFn     = capture.Window
	captureRegionFn     = capture.Region
	captureRegionRectFn = capture.RegionRect
)
