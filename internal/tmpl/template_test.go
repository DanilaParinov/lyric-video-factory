package tmpl

import (
	"strings"
	"testing"
)

func TestEscapeText(t *testing.T) {
	cases := []struct{ in, want string }{
		{"Hello world", "Hello world"},
		{"it's", `it\'s`},
		{`back\slash`, `back\\slash`},
		{`both\and'here`, `both\\and\'here`},
		{"", ""},
	}
	for _, c := range cases {
		if got := EscapeText(c.in); got != c.want {
			t.Errorf("EscapeText(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestParsePercent(t *testing.T) {
	valid := []struct {
		s    string
		want float64
	}{
		{"0%", 0},
		{"50%", 50},
		{"100%", 100},
		{"  75%  ", 75},
	}
	for _, c := range valid {
		v, err := parsePercent(c.s)
		if err != nil || v != c.want {
			t.Errorf("parsePercent(%q) = %v, %v; want %v, nil", c.s, v, err, c.want)
		}
	}

	invalid := []string{"50", "abc%", "-1%", "101%", ""}
	for _, s := range invalid {
		if _, err := parsePercent(s); err == nil {
			t.Errorf("parsePercent(%q): expected error, got nil", s)
		}
	}
}

func TestApplyDefaults_emptyTemplate(t *testing.T) {
	tpl := &Template{}
	ApplyDefaults(tpl)
	if tpl.Font.File == "" {
		t.Error("Font.File should be set by defaults")
	}
	if tpl.Font.Color == "" {
		t.Error("Font.Color should be set by defaults")
	}
	if tpl.Font.Size <= 0 {
		t.Error("Font.Size should be positive after defaults")
	}
	if tpl.TextBaselineY == "" {
		t.Error("TextBaselineY should be set by defaults")
	}
}

func TestApplyDefaults_preservesExistingValues(t *testing.T) {
	tpl := &Template{
		Font:          FontStyle{File: "custom.ttf", Color: "#ff0000", Size: 48},
		TextBaselineY: "30%",
	}
	ApplyDefaults(tpl)
	if tpl.Font.File != "custom.ttf" {
		t.Errorf("Font.File changed to %q", tpl.Font.File)
	}
	if tpl.Font.Color != "#ff0000" {
		t.Errorf("Font.Color changed to %q", tpl.Font.Color)
	}
	if tpl.Font.Size != 48 {
		t.Errorf("Font.Size changed to %d", tpl.Font.Size)
	}
	if tpl.TextBaselineY != "30%" {
		t.Errorf("TextBaselineY changed to %q", tpl.TextBaselineY)
	}
}

func validTemplate() *Template {
	return &Template{
		Width:         1920,
		Height:        1080,
		Font:          FontStyle{File: "font.ttf", Size: 72},
		TextBaselineY: "50%",
		Texts:         []TextCue{{Text: "Hello", Start: 0, End: 3}},
	}
}

func TestValidate_valid(t *testing.T) {
	if err := validTemplate().Validate(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidate_zeroDimensions(t *testing.T) {
	tpl := validTemplate()
	tpl.Width = 0
	if err := tpl.Validate(); err == nil {
		t.Error("expected error for zero width")
	}
	tpl2 := validTemplate()
	tpl2.Height = 0
	if err := tpl2.Validate(); err == nil {
		t.Error("expected error for zero height")
	}
}

func TestValidate_emptyFontFile(t *testing.T) {
	tpl := validTemplate()
	tpl.Font.File = ""
	if err := tpl.Validate(); err == nil {
		t.Error("expected error for empty font file")
	}
}

func TestValidate_zeroFontSize(t *testing.T) {
	tpl := validTemplate()
	tpl.Font.Size = 0
	if err := tpl.Validate(); err == nil {
		t.Error("expected error for zero font size")
	}
}

func TestValidate_invalidBaselineY(t *testing.T) {
	tpl := validTemplate()
	tpl.TextBaselineY = "not-a-percent"
	if err := tpl.Validate(); err == nil {
		t.Error("expected error for invalid baseline y")
	}
}

func TestValidate_emptyTexts(t *testing.T) {
	tpl := validTemplate()
	tpl.Texts = nil
	if err := tpl.Validate(); err == nil {
		t.Error("expected error for empty texts")
	}
}

func TestValidate_emptyTextString(t *testing.T) {
	tpl := validTemplate()
	tpl.Texts = []TextCue{{Text: "", Start: 0, End: 3}}
	if err := tpl.Validate(); err == nil {
		t.Error("expected error for empty text string")
	}
}

func TestValidate_textStartGeEnd(t *testing.T) {
	tpl := validTemplate()
	tpl.Texts = []TextCue{{Text: "Hi", Start: 3, End: 3}}
	if err := tpl.Validate(); err == nil {
		t.Error("expected error for text start >= end")
	}
}

func TestValidate_segmentStartGeEnd(t *testing.T) {
	tpl := validTemplate()
	tpl.VideoSegments = []VideoSegment{{Start: 5, End: 3}}
	if err := tpl.Validate(); err == nil {
		t.Error("expected error for segment start >= end")
	}
}

func TestParseData_nativeFormat(t *testing.T) {
	raw := `{
		"width": 1920,
		"height": 1080,
		"font": {"file": "font.ttf", "size": 72, "color": "#fff"},
		"text_baseline_y": "50%",
		"texts": [{"text": "Hello", "start": 0, "end": 3}]
	}`
	tpl, err := ParseData([]byte(raw))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tpl.Width != 1920 {
		t.Errorf("Width = %d, want 1920", tpl.Width)
	}
	if len(tpl.Texts) != 1 || tpl.Texts[0].Text != "Hello" {
		t.Errorf("Texts = %+v", tpl.Texts)
	}
}

func TestParseData_invalidJSON(t *testing.T) {
	if _, err := ParseData([]byte(`{invalid}`)); err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestBaselineYExpr(t *testing.T) {
	tpl := &Template{TextBaselineY: "50%"}
	expr := tpl.BaselineYExpr()
	if !strings.Contains(expr, "h*") {
		t.Errorf("BaselineYExpr = %q, expected to contain h*", expr)
	}
	if !strings.Contains(expr, "ascent") {
		t.Errorf("BaselineYExpr = %q, expected to contain ascent", expr)
	}
}
