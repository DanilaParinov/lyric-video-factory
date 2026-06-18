package render

import (
	"strings"
	"testing"

	"lyric-video-factory/internal/tmpl"
)

func TestLastLines(t *testing.T) {
	cases := []struct {
		s    string
		n    int
		want string
	}{
		{"a\nb\nc\nd", 2, "c\nd"},
		{"a\nb\nc", 5, "a\nb\nc"},
		{"single", 1, "single"},
		{"", 5, ""},
		{"  \n  ", 5, ""},
	}
	for _, c := range cases {
		if got := lastLines(c.s, c.n); got != c.want {
			t.Errorf("lastLines(%q, %d) = %q, want %q", c.s, c.n, got, c.want)
		}
	}
}

func TestTotalDuration_drivenByTexts(t *testing.T) {
	tpl := &tmpl.Template{
		Texts: []tmpl.TextCue{
			{Text: "A", Start: 0, End: 5},
			{Text: "B", Start: 3, End: 8},
		},
	}
	if d := totalDuration(tpl); d != 8 {
		t.Errorf("totalDuration = %v, want 8", d)
	}
}

func TestTotalDuration_drivenBySegments(t *testing.T) {
	tpl := &tmpl.Template{
		Texts:         []tmpl.TextCue{{Text: "A", Start: 0, End: 5}},
		VideoSegments: []tmpl.VideoSegment{{Start: 0, End: 10}},
	}
	if d := totalDuration(tpl); d != 10 {
		t.Errorf("totalDuration = %v, want 10", d)
	}
}

func TestTotalDuration_empty(t *testing.T) {
	if d := totalDuration(&tmpl.Template{}); d != 0 {
		t.Errorf("totalDuration = %v, want 0", d)
	}
}

func TestBuildTimeline_noGaps(t *testing.T) {
	tpl := &tmpl.Template{
		Texts:         []tmpl.TextCue{{Text: "A", Start: 0, End: 5}},
		VideoSegments: []tmpl.VideoSegment{{Start: 0, End: 5}},
	}
	segs := []picked{{path: "clip.mp4", duration: 5}}
	tl := buildTimeline(tpl, segs)

	if len(tl) != 1 {
		t.Fatalf("timeline len = %d, want 1", len(tl))
	}
	if tl[0].isBlack {
		t.Error("expected video segment, got black")
	}
	if tl[0].duration != 5 {
		t.Errorf("duration = %v, want 5", tl[0].duration)
	}
}

func TestBuildTimeline_withGaps(t *testing.T) {
	// intro (0–2), video (2–5), outro (5–8)
	tpl := &tmpl.Template{
		Texts:         []tmpl.TextCue{{Text: "A", Start: 0, End: 8}},
		VideoSegments: []tmpl.VideoSegment{{Start: 2, End: 5}},
	}
	segs := []picked{{path: "clip.mp4", duration: 3}}
	tl := buildTimeline(tpl, segs)

	if len(tl) != 3 {
		t.Fatalf("timeline len = %d, want 3 (black+video+black)", len(tl))
	}
	if !tl[0].isBlack || tl[0].duration != 2 {
		t.Errorf("tl[0] = %+v, want black duration=2", tl[0])
	}
	if tl[1].isBlack {
		t.Errorf("tl[1] should be video, got black")
	}
	if !tl[2].isBlack || tl[2].duration != 3 {
		t.Errorf("tl[2] = %+v, want black duration=3", tl[2])
	}
}

func TestBuildTimeline_emptySegments(t *testing.T) {
	tpl := &tmpl.Template{
		Texts: []tmpl.TextCue{{Text: "A", Start: 0, End: 5}},
	}
	tl := buildTimeline(tpl, nil)

	if len(tl) != 1 {
		t.Fatalf("timeline len = %d, want 1", len(tl))
	}
	if !tl[0].isBlack || tl[0].duration != 5 {
		t.Errorf("tl[0] = %+v, want black duration=5", tl[0])
	}
}

func TestPickSegments_durations(t *testing.T) {
	tpl := &tmpl.Template{
		VideoSegments: []tmpl.VideoSegment{
			{Start: 0, End: 3},
			{Start: 3, End: 7},
		},
	}
	pool := &Pool{
		Clips: []Clip{
			{Path: "a.mp4", Duration: 10},
			{Path: "b.mp4", Duration: 10},
		},
	}
	segs := pickSegments(tpl, pool)

	if len(segs) != 2 {
		t.Fatalf("pickSegments len = %d, want 2", len(segs))
	}
	if segs[0].duration != 3 {
		t.Errorf("segs[0].duration = %v, want 3", segs[0].duration)
	}
	if segs[1].duration != 4 {
		t.Errorf("segs[1].duration = %v, want 4", segs[1].duration)
	}
}

func TestPickSegments_loopWhenClipShorter(t *testing.T) {
	tpl := &tmpl.Template{
		VideoSegments: []tmpl.VideoSegment{{Start: 0, End: 10}},
	}
	pool := &Pool{Clips: []Clip{{Path: "short.mp4", Duration: 3}}}
	segs := pickSegments(tpl, pool)
	if !segs[0].loop {
		t.Error("expected loop=true when clip is shorter than segment")
	}
}

func TestPickSegments_noLoopWhenClipLonger(t *testing.T) {
	tpl := &tmpl.Template{
		VideoSegments: []tmpl.VideoSegment{{Start: 0, End: 3}},
	}
	pool := &Pool{Clips: []Clip{{Path: "long.mp4", Duration: 10}}}
	segs := pickSegments(tpl, pool)
	if segs[0].loop {
		t.Error("expected loop=false when clip is longer than segment")
	}
}

func TestBuildFC_containsDrawtext(t *testing.T) {
	tpl := &tmpl.Template{
		Width:         1920,
		Height:        1080,
		Font:          tmpl.FontStyle{File: "font.ttf", Size: 72, Color: "#ffffff"},
		TextBaselineY: "50%",
		Texts:         []tmpl.TextCue{{Text: "Hello", Start: 1, End: 3}},
	}
	tl := []timelineSeg{{isBlack: false, duration: 5, path: "clip.mp4"}}
	fc := buildFC(tpl, tl)

	if !strings.Contains(fc, "drawtext") {
		t.Error("buildFC output missing drawtext filter")
	}
	if !strings.Contains(fc, "Hello") {
		t.Error("buildFC output missing text content")
	}
	if !strings.Contains(fc, "concat") {
		t.Error("buildFC output missing concat filter")
	}
}

func TestBuildFC_dimLevelAddsColorchannelmixer(t *testing.T) {
	tpl := &tmpl.Template{
		Width:         1920,
		Height:        1080,
		Font:          tmpl.FontStyle{File: "font.ttf", Size: 72, Color: "#ffffff"},
		TextBaselineY: "50%",
		DimLevel:      0.5,
		Texts:         []tmpl.TextCue{{Text: "Hi", Start: 0, End: 3}},
	}
	tl := []timelineSeg{{isBlack: false, duration: 3, path: "clip.mp4"}}
	fc := buildFC(tpl, tl)

	if !strings.Contains(fc, "colorchannelmixer") {
		t.Error("buildFC with DimLevel should contain colorchannelmixer")
	}
}

func TestBuildFC_noDimLevel(t *testing.T) {
	tpl := &tmpl.Template{
		Width:         1920,
		Height:        1080,
		Font:          tmpl.FontStyle{File: "font.ttf", Size: 72, Color: "#ffffff"},
		TextBaselineY: "50%",
		DimLevel:      0,
		Texts:         []tmpl.TextCue{{Text: "Hi", Start: 0, End: 3}},
	}
	tl := []timelineSeg{{isBlack: false, duration: 3, path: "clip.mp4"}}
	fc := buildFC(tpl, tl)

	if strings.Contains(fc, "colorchannelmixer") {
		t.Error("buildFC with DimLevel=0 should not contain colorchannelmixer")
	}
}
