package tmpl

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// cmDoc is the minimal Creatomate JSON structure needed for import.
type cmDoc struct {
	Width    int         `json:"width"`
	Height   int         `json:"height"`
	Elements []cmElement `json:"elements"`
}

type cmElement struct {
	Type      string   `json:"type"`
	Track     int      `json:"track"`
	Time      *float64 `json:"time"` // nil = follows immediately after the previous element on the same track
	Duration  float64  `json:"duration"`
	Text      string   `json:"text"`
	Y         string   `json:"y"`
	FontSize  string   `json:"font_size"`
	FillColor string   `json:"fill_color"`
	// shape fields
	Width   string `json:"width"`
	Height  string `json:"height"`
	Opacity string `json:"opacity"`
}

// ImportCreatomate reads a Creatomate JSON file and converts it to a native Template.
//
// Imported: canvas size, text and video-segment timings, text position, font color and size.
// Not imported: source UUIDs (no local file), audio, shape elements.
//
// After import, Font.File and Audio must be filled in manually.
func ImportCreatomate(path string) (*Template, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("читаю файл: %w", err)
	}
	var doc cmDoc
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("парсю Creatomate JSON: %w", err)
	}
	return convertCreatomate(&doc)
}

func convertCreatomate(doc *cmDoc) (*Template, error) {
	t := &Template{
		Width:  doc.Width,
		Height: doc.Height,
	}

	// Accumulated time per track: if an element has no time field,
	// it starts where the previous element on the same track ended.
	trackCursor := map[int]float64{}
	var firstText *cmElement

	for i := range doc.Elements {
		el := &doc.Elements[i]

		var start float64
		if el.Time != nil {
			start = *el.Time
		} else {
			start = trackCursor[el.Track]
		}
		end := start + el.Duration
		trackCursor[el.Track] = end

		switch el.Type {
		case "video":
			t.VideoSegments = append(t.VideoSegments, VideoSegment{Start: start, End: end})

		case "text":
			if firstText == nil {
				firstText = el
			}
			t.Texts = append(t.Texts, TextCue{Text: el.Text, Start: start, End: end})

		case "shape":
			// Full-screen rectangle with opacity — dim overlay
			if el.Width == "100%" && el.Height == "100%" && el.Opacity != "" {
				if v, err := parsePercent(el.Opacity); err == nil {
					t.DimLevel = v / 100
				}
			}
		}
		// "audio" — skipped
	}

	// Style and position from the first text element
	if firstText != nil {
		// In Creatomate y_anchor="0%" means y is the top of the bounding box.
		// We store it as text_baseline_y (approximation); the user can adjust manually if needed.
		t.TextBaselineY = firstText.Y
		t.Font.Color = firstText.FillColor
		if sz, err := parseVmin(firstText.FontSize, doc.Width, doc.Height); err == nil {
			t.Font.Size = sz
		}
	} else {
		t.TextBaselineY = "50%"
	}

	return t, nil
}

// parseVmin converts a CSS value like "5 vmin" to pixels relative to the canvas.
// Also accepts a plain number, which is treated as pixels.
func parseVmin(s string, w, h int) (int, error) {
	s = strings.TrimSpace(s)
	if strings.HasSuffix(s, "vmin") {
		numStr := strings.TrimSpace(strings.TrimSuffix(s, "vmin"))
		v, err := strconv.ParseFloat(numStr, 64)
		if err != nil {
			return 0, err
		}
		vmin := w
		if h < vmin {
			vmin = h
		}
		return int(v * float64(vmin) / 100), nil
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, fmt.Errorf("неизвестный формат font_size: %q", s)
	}
	return int(v), nil
}
