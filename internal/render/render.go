package render

import (
	"bytes"
	"fmt"
	"log"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"lyric-video-factory/internal/tmpl"
)

type Job struct {
	Template    *tmpl.Template
	Pool        *Pool
	N           int
	OutputDir   string
	Concurrency int // 0 = runtime.NumCPU()
}

// Run генерирует N вариантов параллельно.
func Run(job *Job) error {
	if err := os.MkdirAll(job.OutputDir, 0755); err != nil {
		return fmt.Errorf("создаю output dir: %w", err)
	}

	concurrency := job.Concurrency
	if concurrency <= 0 {
		concurrency = runtime.NumCPU()
	}
	if concurrency > job.N {
		concurrency = job.N
	}

	sem := make(chan struct{}, concurrency)
	errs := make(chan error, job.N)
	var wg sync.WaitGroup

	for i := 1; i <= job.N; i++ {
		wg.Add(1)
		go func(num int) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			outPath := filepath.Join(job.OutputDir, fmt.Sprintf("variant_%02d.mp4", num))
			log.Printf("[%d/%d] генерирую %s", num, job.N, outPath)

			if err := renderVariant(job.Template, job.Pool, outPath); err != nil {
				errs <- fmt.Errorf("вариант %d: %w", num, err)
				return
			}
			log.Printf("[%d/%d] готово  %s", num, job.N, outPath)
		}(i)
	}

	wg.Wait()
	close(errs)

	for err := range errs {
		return err
	}
	return nil
}

// ---

type picked struct {
	path     string
	duration float64
	loop     bool // клип короче сегмента — нужен зацикленный ввод
}

func renderVariant(t *tmpl.Template, pool *Pool, outPath string) error {
	if t.Audio != "" {
		if _, err := os.Stat(t.Audio); err != nil {
			return fmt.Errorf("аудиофайл не найден: %s", t.Audio)
		}
	}
	if _, err := os.Stat(t.Font.File); err != nil {
		return fmt.Errorf("файл шрифта не найден: %s — положите его рядом с бинарником или укажите полный путь", t.Font.File)
	}

	segs := pickSegments(t, pool)
	args := buildArgs(t, segs, outPath)

	cmd := exec.Command("ffmpeg", args...)
	var errBuf bytes.Buffer
	cmd.Stderr = &errBuf

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("FFmpeg: %w\n%s", err, lastLines(errBuf.String(), 20))
	}
	return nil
}

// lastLines возвращает последние n строк из s (хвост stderr обычно содержит суть ошибки).
func lastLines(s string, n int) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	lines := strings.Split(s, "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}

// pickSegments случайно распределяет клипы из пула по сегментам шаблона.
// Клипы перемешиваются, чтобы в одном варианте реже повторялись одинаковые.
func pickSegments(t *tmpl.Template, pool *Pool) []picked {
	order := rand.Perm(len(pool.Clips))
	segs := make([]picked, len(t.VideoSegments))
	for i, vs := range t.VideoSegments {
		dur := vs.End - vs.Start
		clip := pool.Clips[order[i%len(pool.Clips)]]
		segs[i] = picked{path: clip.Path, duration: dur, loop: clip.Duration < dur}
	}
	return segs
}

// ---

type timelineSeg struct {
	isBlack  bool
	duration float64
	path     string
	loop     bool
}

func totalDuration(t *tmpl.Template) float64 {
	var d float64
	for _, tc := range t.Texts {
		if tc.End > d {
			d = tc.End
		}
	}
	for _, vs := range t.VideoSegments {
		if vs.End > d {
			d = vs.End
		}
	}
	return d
}

// buildTimeline строит полный список сегментов от 0 до totalDuration,
// заполняя пробелы (интро, аутро, зазоры между сегментами) чёрным фоном.
func buildTimeline(t *tmpl.Template, segs []picked) []timelineSeg {
	totalDur := totalDuration(t)
	var tl []timelineSeg
	cursor := 0.0

	for i, vs := range t.VideoSegments {
		if vs.Start > cursor+0.001 {
			tl = append(tl, timelineSeg{isBlack: true, duration: vs.Start - cursor})
		}
		tl = append(tl, timelineSeg{
			duration: segs[i].duration,
			path:     segs[i].path,
			loop:     segs[i].loop,
		})
		cursor = vs.End
	}

	if totalDur > cursor+0.001 {
		tl = append(tl, timelineSeg{isBlack: true, duration: totalDur - cursor})
	}
	return tl
}

// ---

func buildArgs(t *tmpl.Template, segs []picked, outPath string) []string {
	tl := buildTimeline(t, segs)
	totalDur := totalDuration(t)

	args := []string{"-y", "-loglevel", "error"}

	// Входные файлы: чёрные клипы через lavfi, видео-клипы с seek-offset
	for _, s := range tl {
		if s.isBlack {
			args = append(args,
				"-f", "lavfi",
				"-i", fmt.Sprintf("color=c=black:s=%dx%d:r=30:d=%.4f", t.Width, t.Height, s.duration),
			)
		} else {
			if s.loop {
				args = append(args, "-stream_loop", "-1")
			}
			args = append(args, "-i", s.path)
		}
	}

	audioIdx := len(tl)
	if t.Audio != "" {
		args = append(args, "-i", t.Audio)
	}

	args = append(args, "-filter_complex", buildFC(t, tl))
	args = append(args, "-map", "[out]")

	if t.Audio != "" {
		args = append(args,
			"-map", fmt.Sprintf("%d:a", audioIdx),
			"-c:a", "aac", "-b:a", "192k",
		)
	}

	args = append(args,
		"-c:v", "libx264", "-pix_fmt", "yuv420p", "-preset", "fast", "-crf", "23",
		"-t", fmt.Sprintf("%.4f", totalDur),
		outPath,
	)
	return args
}

// buildFC строит строку filter_complex:
// 1. нормализация каждого сегмента (обрезка по длине, масштаб, fps, SAR)
// 2. concat всех сегментов в [base]
// 3. цепочка drawtext с enable-выражениями по таймингам лирики
func buildFC(t *tmpl.Template, tl []timelineSeg) string {
	var sb strings.Builder
	labels := make([]string, len(tl))

	for i, s := range tl {
		label := fmt.Sprintf("[s%d]", i)
		labels[i] = label

		if s.isBlack {
			// Чёрный клип: только обрезаем по длине и нормализуем
			fmt.Fprintf(&sb,
				"[%d:v]trim=duration=%.4f,fps=30,setsar=1%s;\n",
				i, s.duration, label)
		} else {
			// Видеоклип: обрезаем, масштабируем в размер холста с паддингом (letterbox)
			fmt.Fprintf(&sb,
				"[%d:v]trim=duration=%.4f,setpts=PTS-STARTPTS,fps=30,scale=%d:%d:force_original_aspect_ratio=decrease,pad=%d:%d:(ow-iw)/2:(oh-ih)/2:black,setsar=1%s;\n",
				i, s.duration, t.Width, t.Height, t.Width, t.Height, label)
		}
	}

	// concat всех сегментов
	sb.WriteString(strings.Join(labels, ""))
	fmt.Fprintf(&sb, "concat=n=%d:v=1:a=0[base];\n", len(tl))

	// Затемнение поверх видео (colorchannelmixer умножает каждый канал на 1-dim)
	videoOut := "[base]"
	if t.DimLevel > 0 {
		m := 1.0 - t.DimLevel
		fmt.Fprintf(&sb, "[base]colorchannelmixer=rr=%.6f:gg=%.6f:bb=%.6f[dimmed];\n", m, m, m)
		videoOut = "[dimmed]"
	}

	// drawtext-цепочка: каждый текст активен в своём временном окне
	// enable=gte(t\,START)*lte(t\,END) — \, экранирует запятую в парсере FFmpeg-фильтров
	baseY := t.BaselineYExpr()
	sb.WriteString(videoOut)
	for i, tc := range t.Texts {
		if i > 0 {
			sb.WriteString(",")
		}
		enable := fmt.Sprintf("gte(t\\,%.4f)*lte(t\\,%.4f)", tc.Start, tc.End)
		fmt.Fprintf(&sb,
			"drawtext=fontfile=%s:text='%s':fontsize=%d:fontcolor=%s:x=(w-text_w)/2:y=%s:enable=%s",
			t.Font.File, tmpl.EscapeText(tc.Text), t.Font.Size, t.Font.Color, baseY, enable,
		)
	}
	sb.WriteString("[out]")

	return sb.String()
}
