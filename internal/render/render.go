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

// Run generates N variants in parallel.
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
	loop     bool // clip is shorter than the segment — looped input required
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

// lastLines returns the last n lines of s (stderr tail usually contains the root cause).
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

// pickSegments randomly distributes pool clips across the template's video segments.
// Clips are shuffled so the same clip is less likely to repeat within one variant.
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

// buildTimeline builds the full segment list from 0 to totalDuration,
// filling gaps (intro, outro, gaps between segments) with black.
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

	// Inputs: black clips via lavfi, video clips with seek offset
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

// buildFC builds the filter_complex string:
// 1. normalise each segment (trim to length, scale, fps, SAR)
// 2. concat all segments into [base]
// 3. drawtext chain with enable expressions keyed to lyric timings
func buildFC(t *tmpl.Template, tl []timelineSeg) string {
	var sb strings.Builder
	labels := make([]string, len(tl))

	for i, s := range tl {
		label := fmt.Sprintf("[s%d]", i)
		labels[i] = label

		if s.isBlack {
			// Black clip: trim to length and normalise
			fmt.Fprintf(&sb,
				"[%d:v]trim=duration=%.4f,fps=30,setsar=1%s;\n",
				i, s.duration, label)
		} else {
			// Video clip: trim, scale to canvas with letterbox padding
			fmt.Fprintf(&sb,
				"[%d:v]trim=duration=%.4f,setpts=PTS-STARTPTS,fps=30,scale=%d:%d:force_original_aspect_ratio=decrease,pad=%d:%d:(ow-iw)/2:(oh-ih)/2:black,setsar=1%s;\n",
				i, s.duration, t.Width, t.Height, t.Width, t.Height, label)
		}
	}

	// concat all segments
	sb.WriteString(strings.Join(labels, ""))
	fmt.Fprintf(&sb, "concat=n=%d:v=1:a=0[base];\n", len(tl))

	// Dim overlay (colorchannelmixer multiplies each channel by 1-dim)
	videoOut := "[base]"
	if t.DimLevel > 0 {
		m := 1.0 - t.DimLevel
		fmt.Fprintf(&sb, "[base]colorchannelmixer=rr=%.6f:gg=%.6f:bb=%.6f[dimmed];\n", m, m, m)
		videoOut = "[dimmed]"
	}

	// drawtext chain: each cue is active within its time window
	// enable=gte(t\,START)*lte(t\,END) — \, escapes the comma in FFmpeg's filter parser
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
