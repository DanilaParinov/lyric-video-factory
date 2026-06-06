package tmpl

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type Template struct {
	Audio         string         `json:"audio"`
	Width         int            `json:"width"`
	Height        int            `json:"height"`
	Font          FontStyle      `json:"font"`
	TextBaselineY string         `json:"text_baseline_y"` // "50%" — позиция базовой линии
	DimLevel      float64        `json:"dim_level"`        // 0.0–1.0; затемнение поверх видео (под текстом)
	VideoSegments []VideoSegment `json:"video_segments"`
	Texts         []TextCue      `json:"texts"`
}

type FontStyle struct {
	File  string `json:"file"`
	Size  int    `json:"size"`
	Color string `json:"color"`
}

type VideoSegment struct {
	Start float64 `json:"start"`
	End   float64 `json:"end"`
}

type TextCue struct {
	Text  string  `json:"text"`
	Start float64 `json:"start"`
	End   float64 `json:"end"`
}

func Parse(path string) (*Template, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("читаю шаблон: %w", err)
	}

	t, err := parseAuto(data)
	if err != nil {
		return nil, err
	}

	ApplyDefaults(t)

	if err := t.Validate(); err != nil {
		return nil, fmt.Errorf("валидация шаблона: %w", err)
	}
	return t, nil
}

// parseAuto определяет формат JSON (нативный vs Creatomate) и парсит соответственно.
func parseAuto(data []byte) (*Template, error) {
	var probe struct {
		Elements json.RawMessage `json:"elements"`
	}
	_ = json.Unmarshal(data, &probe)

	if probe.Elements != nil {
		var doc cmDoc
		if err := json.Unmarshal(data, &doc); err != nil {
			return nil, fmt.Errorf("парсю Creatomate JSON: %w", err)
		}
		return convertCreatomate(&doc)
	}

	var t Template
	if err := json.Unmarshal(data, &t); err != nil {
		return nil, fmt.Errorf("парсю шаблон: %w", err)
	}
	return &t, nil
}

// ApplyDefaults подставляет разумные значения для незаполненных полей.
// Используется при импорте Creatomate и при парсинге нативных шаблонов без явных значений.
func ApplyDefaults(t *Template) {
	if t.Font.File == "" {
		t.Font.File = "SF-Pro-Display-Thin.otf"
	}
	if t.Font.Color == "" {
		t.Font.Color = "#ffffff"
	}
	if t.Font.Size <= 0 {
		t.Font.Size = 72
	}
	if t.TextBaselineY == "" {
		t.TextBaselineY = "50%"
	}
}

func (t *Template) Validate() error {
	if t.Width <= 0 || t.Height <= 0 {
		return fmt.Errorf("width и height должны быть положительными")
	}
	if t.Font.File == "" {
		return fmt.Errorf("font.file не задан")
	}
	if t.Font.Size <= 0 {
		return fmt.Errorf("font.size должен быть положительным")
	}
	if _, err := parsePercent(t.TextBaselineY); err != nil {
		return fmt.Errorf("text_baseline_y: %w", err)
	}
	if len(t.Texts) == 0 {
		return fmt.Errorf("texts пустой")
	}
	for i, tc := range t.Texts {
		if tc.Text == "" {
			return fmt.Errorf("texts[%d]: пустой текст", i)
		}
		if tc.Start >= tc.End {
			return fmt.Errorf("texts[%d] %q: start >= end", i, tc.Text)
		}
	}
	for i, vs := range t.VideoSegments {
		if vs.Start >= vs.End {
			return fmt.Errorf("video_segments[%d]: start >= end", i)
		}
	}
	return nil
}

// BaselineYExpr возвращает FFmpeg-выражение для фиксированной базовой линии.
// y=h*pct-ascent держит baseline строго на одной высоте для любой строки.
func (t *Template) BaselineYExpr() string {
	pct, _ := parsePercent(t.TextBaselineY)
	return fmt.Sprintf("h*%.6f-ascent", pct/100)
}

// EscapeText экранирует строку для использования в одинарных кавычках drawtext.
func EscapeText(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `'`, `\'`)
	return s
}

var audioExts = map[string]bool{
	".wav": true, ".mp3": true, ".aac": true, ".m4a": true, ".ogg": true, ".flac": true,
}

// FindAudio возвращает путь к первому найденному аудиофайлу в переданных директориях.
// Директории проверяются по порядку — первая с результатом побеждает.
func FindAudio(dirs ...string) string {
	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !e.IsDir() && audioExts[strings.ToLower(filepath.Ext(e.Name()))] {
				return filepath.Join(dir, e.Name())
			}
		}
	}
	return ""
}

func parsePercent(s string) (float64, error) {
	s = strings.TrimSpace(s)
	if !strings.HasSuffix(s, "%") {
		return 0, fmt.Errorf("ожидается значение вида \"50%%\", получено %q", s)
	}
	v, err := strconv.ParseFloat(strings.TrimSuffix(s, "%"), 64)
	if err != nil {
		return 0, fmt.Errorf("не число: %w", err)
	}
	if v < 0 || v > 100 {
		return 0, fmt.Errorf("вне диапазона 0–100: %v", v)
	}
	return v, nil
}
