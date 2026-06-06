package render

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

type Clip struct {
	Path     string
	Duration float64
}

type Pool struct {
	Clips []Clip
}

var videoExts = map[string]bool{
	".mp4": true, ".mov": true, ".avi": true, ".mkv": true,
	".webm": true, ".m4v": true, ".flv": true, ".wmv": true,
}

// LoadPool сканирует dir, берёт все видеофайлы и измеряет длительность через ffprobe.
func LoadPool(dir string) (*Pool, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("читаю пул %s: %w", dir, err)
	}
	p := &Pool{}
	for _, e := range entries {
		if e.IsDir() || !videoExts[strings.ToLower(filepath.Ext(e.Name()))] {
			continue
		}
		path := filepath.Join(dir, e.Name())
		dur, err := ProbeDuration(path)
		if err != nil {
			return nil, fmt.Errorf("ffprobe %s: %w", e.Name(), err)
		}
		p.Clips = append(p.Clips, Clip{Path: path, Duration: dur})
	}
	if len(p.Clips) == 0 {
		return nil, fmt.Errorf("нет видеофайлов в %s", dir)
	}
	return p, nil
}

type ffprobeOut struct {
	Format struct {
		Duration string `json:"duration"`
	} `json:"format"`
}

// ProbeDuration возвращает длительность видеофайла в секундах через ffprobe.
func ProbeDuration(path string) (float64, error) {
	out, err := exec.Command("ffprobe",
		"-v", "quiet",
		"-print_format", "json",
		"-show_entries", "format=duration",
		path,
	).Output()
	if err != nil {
		return 0, err
	}
	var res ffprobeOut
	if err := json.Unmarshal(out, &res); err != nil {
		return 0, err
	}
	return strconv.ParseFloat(res.Format.Duration, 64)
}
