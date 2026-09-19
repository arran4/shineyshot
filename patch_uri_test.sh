#!/bin/bash
cat << 'INNER_EOF' > /tmp/uri_test_patch.go
func TestPortalScreenshotURIParsing(t *testing.T) {
	tempDir := t.TempDir()
	path1 := filepath.Join(tempDir, "example.png")
	path2 := filepath.Join(tempDir, "Shiney Shot.png")

	// Create tiny PNGs
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	writePNG := func(p string) {
		f, err := os.Create(p)
		if err != nil {
			t.Fatalf("failed to create temp png %q: %v", p, err)
		}
		defer f.Close()
		if err := png.Encode(f, img); err != nil {
			t.Fatalf("failed to encode png to %q: %v", p, err)
		}
	}
	writePNG(path1)
	writePNG(path2)

	tests := []struct {
		name      string
		uri       string
		wantError string
		wantPath  string
	}{
		{
			name:      "ordinary file uri",
			uri:       "file://" + path1,
			wantError: "",
			wantPath:  path1,
		},
		{
			name:      "percent-escaped local pathname",
			uri:       "file://" + strings.ReplaceAll(path2, " ", "%20"),
			wantError: "",
			wantPath:  path2,
		},
		{
			name:      "malformed uri",
			uri:       "://invalid",
			wantError: "invalid uri",
		},
		{
			name:      "unsupported scheme",
			uri:       "http://example.com/image.png",
			wantError: "unsupported uri scheme \"http\"",
		},
		{
			name:      "unsupported host",
			uri:       "file://remotehost/tmp/image.png",
			wantError: "unsupported uri host \"remotehost\"",
		},
		{
			name:      "localhost host",
			uri:       "file://localhost" + path1,
			wantError: "",
			wantPath:  path1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Write the file again just in case previous runs deleted it (since loadPNG removes the file)
			if tc.wantPath != "" {
				writePNG(tc.wantPath)
			}

			bus := &mockPortalBus{
				uniqueName: ":1.123",
			}
			expectedPath := dbus.ObjectPath("/org/freedesktop/portal/desktop/request/1_123/test_token")

			bus.callFunc = func(dest string, path dbus.ObjectPath, method string, flags dbus.Flags, args ...any) (*dbus.Call, error) {
				if bus.signalChan != nil {
					bus.signalChan <- &dbus.Signal{
						Path: expectedPath,
						Name: "org.freedesktop.portal.Request.Response",
						Body: []any{
							uint32(0),
							map[string]dbus.Variant{
								"uri": dbus.MakeVariant(tc.uri),
							},
						},
					}
				}
				return &dbus.Call{
					Body: []any{expectedPath},
				}, nil
			}

			prevConnect := connectPortalBus
			prevToken := portalHandleToken
			connectPortalBus = func() (portalBus, error) {
				return bus, nil
			}
			portalHandleToken = func() (string, error) {
				return "test_token", nil
			}
			t.Cleanup(func() {
				connectPortalBus = prevConnect
				portalHandleToken = prevToken
			})

			res, err := portalScreenshot(false, Options{})
			if tc.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantError) {
					t.Fatalf("expected error containing %q, got %v", tc.wantError, err)
				}
			} else {
				if err != nil {
					t.Fatalf("expected success, got error: %v", err)
				}
				if res == nil {
					t.Fatalf("expected image, got nil")
				}
			}
		})
	}
}
INNER_EOF
