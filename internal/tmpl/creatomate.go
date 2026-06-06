package tmpl

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// cmDoc — минимальная структура Creatomate JSON для импорта.
type cmDoc struct {
	Width    int         `json:"width"`
	Height   int         `json:"height"`
	Elements []cmElement `json:"elements"`
}

type cmElement struct {
	Type      string   `json:"type"`
	Track     int      `json:"track"`
	Time      *float64 `json:"time"` // nil = следует сразу за предыдущим элементом трека
	Duration  float64  `json:"duration"`
	Text      string   `json:"text"`
	Y         string   `json:"y"`
	FontSize  string   `json:"font_size"`
	FillColor string   `json:"fill_color"`
	// shape-поля
	Width   string `json:"width"`
	Height  string `json:"height"`
	Opacity string `json:"opacity"`
}

// ImportCreatomate читает Creatomate JSON и конвертирует в нативный Template.
//
// Что берём: размер холста, тайминги текстов и видео-сегментов, позицию текста,
// цвет и размер шрифта.
// Что НЕ берём: source UUID (нет локального файла), audio, shape-элементы.
//
// После импорта нужно вручную заполнить Font.File и Audio.
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

	// Накопленное время по трекам: если у элемента нет поля time,
	// он начинается там, где закончился предыдущий элемент того же трека.
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
			// Полноэкранный прямоугольник с opacity — overlay затемнения
			if el.Width == "100%" && el.Height == "100%" && el.Opacity != "" {
				if v, err := parsePercent(el.Opacity); err == nil {
					t.DimLevel = v / 100
				}
			}
		}
		// "audio" — пропускаем
	}

	// Стиль и позиция из первого text-элемента
	if firstText != nil {
		// В Creatomate y_anchor="0%" означает, что y — верх bounding box.
		// Мы сохраняем как text_baseline_y (приближение); при необходимости
		// пользователь корректирует вручную.
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

// parseVmin конвертирует CSS-значение "5 vmin" → пиксели относительно холста.
// Также принимает чистое число (считается пикселями).
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
