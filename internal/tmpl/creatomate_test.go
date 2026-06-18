package tmpl

import (
	"testing"
)

func TestParseVmin_vminUnit(t *testing.T) {
	// 1920x1080: vmin = 1080; 10vmin = 108px
	v, err := parseVmin("10vmin", 1920, 1080)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v != 108 {
		t.Errorf("parseVmin = %d, want 108", v)
	}
}

func TestParseVmin_vminWithSpace(t *testing.T) {
	v, err := parseVmin("10 vmin", 1920, 1080)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v != 108 {
		t.Errorf("parseVmin = %d, want 108", v)
	}
}

func TestParseVmin_squareCanvas(t *testing.T) {
	// 500x500: vmin = 500; 50vmin = 250px
	v, err := parseVmin("50vmin", 500, 500)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v != 250 {
		t.Errorf("parseVmin = %d, want 250", v)
	}
}

func TestParseVmin_plainPixels(t *testing.T) {
	v, err := parseVmin("72", 1920, 1080)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v != 72 {
		t.Errorf("parseVmin = %d, want 72", v)
	}
}

func TestParseVmin_invalidVmin(t *testing.T) {
	if _, err := parseVmin("abcvmin", 1920, 1080); err == nil {
		t.Error("expected error for non-numeric vmin value")
	}
}

func TestParseVmin_invalidPlain(t *testing.T) {
	if _, err := parseVmin("notanumber", 1920, 1080); err == nil {
		t.Error("expected error for non-numeric plain value")
	}
}

func TestConvertCreatomate_basicMapping(t *testing.T) {
	start1 := 0.0
	start2 := 3.0
	doc := &cmDoc{
		Width:  1920,
		Height: 1080,
		Elements: []cmElement{
			{Type: "video", Track: 1, Time: &start1, Duration: 3},
			{Type: "text", Track: 2, Time: &start2, Duration: 2, Text: "Hello", Y: "50%", FillColor: "#fff", FontSize: "72"},
		},
	}
	tpl, err := convertCreatomate(doc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tpl.Width != 1920 || tpl.Height != 1080 {
		t.Errorf("canvas = %dx%d, want 1920x1080", tpl.Width, tpl.Height)
	}
	if len(tpl.VideoSegments) != 1 {
		t.Fatalf("VideoSegments len = %d, want 1", len(tpl.VideoSegments))
	}
	if tpl.VideoSegments[0].Start != 0 || tpl.VideoSegments[0].End != 3 {
		t.Errorf("VideoSegment = %+v, want {0, 3}", tpl.VideoSegments[0])
	}
	if len(tpl.Texts) != 1 || tpl.Texts[0].Text != "Hello" {
		t.Errorf("Texts = %+v", tpl.Texts)
	}
	if tpl.Texts[0].Start != 3 || tpl.Texts[0].End != 5 {
		t.Errorf("Text timing = [%v, %v], want [3, 5]", tpl.Texts[0].Start, tpl.Texts[0].End)
	}
}

func TestConvertCreatomate_timeNilUsesTrackCursor(t *testing.T) {
	doc := &cmDoc{
		Width:  1280,
		Height: 720,
		Elements: []cmElement{
			{Type: "video", Track: 1, Time: nil, Duration: 5}, // starts at 0
			{Type: "video", Track: 1, Time: nil, Duration: 3}, // starts at 5
		},
	}
	tpl, err := convertCreatomate(doc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tpl.VideoSegments) != 2 {
		t.Fatalf("VideoSegments len = %d, want 2", len(tpl.VideoSegments))
	}
	if tpl.VideoSegments[0].Start != 0 || tpl.VideoSegments[0].End != 5 {
		t.Errorf("segment 0 = %+v, want {0, 5}", tpl.VideoSegments[0])
	}
	if tpl.VideoSegments[1].Start != 5 || tpl.VideoSegments[1].End != 8 {
		t.Errorf("segment 1 = %+v, want {5, 8}", tpl.VideoSegments[1])
	}
}

func TestConvertCreatomate_dimLevel(t *testing.T) {
	start := 0.0
	doc := &cmDoc{
		Width:  1920,
		Height: 1080,
		Elements: []cmElement{
			{Type: "text", Track: 1, Time: &start, Duration: 3, Text: "Hi", Y: "50%"},
			{Type: "shape", Width: "100%", Height: "100%", Opacity: "50%"},
		},
	}
	tpl, err := convertCreatomate(doc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tpl.DimLevel < 0.49 || tpl.DimLevel > 0.51 {
		t.Errorf("DimLevel = %v, want ~0.5", tpl.DimLevel)
	}
}

func TestConvertCreatomate_noTextsFallbackBaseline(t *testing.T) {
	start := 0.0
	doc := &cmDoc{
		Width:  1920,
		Height: 1080,
		Elements: []cmElement{
			{Type: "video", Track: 1, Time: &start, Duration: 5},
		},
	}
	tpl, err := convertCreatomate(doc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tpl.TextBaselineY != "50%" {
		t.Errorf("TextBaselineY = %q, want \"50%%\"", tpl.TextBaselineY)
	}
}
