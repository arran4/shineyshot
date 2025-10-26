package main

import "github.com/example/shineyshot/internal/capture"

var (
	captureScreenshotFn = capture.CaptureScreenshot
	captureWindowFn     = capture.CaptureWindow
	captureRegionFn     = capture.CaptureRegion
	captureRegionRectFn = capture.CaptureRegionRect
)
