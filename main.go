package main

import (
	"embed"
	"fmt"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"

	"lyric-video-factory/internal/render"
	"lyric-video-factory/internal/server"
	"lyric-video-factory/internal/tmpl"
)

//go:embed static
var staticFiles embed.FS

const (
	defaultTemplate = "template.json"
	inputDir        = "input"
	outputDir       = "output"
)

func main() {
	checkBinaries()

	// Без аргументов или с аргументом "serve" — запускаем HTTP-сервер
	if len(os.Args) <= 1 || os.Args[1] == "serve" {
		addr := ":8080"
		if len(os.Args) > 2 {
			addr = os.Args[2]
		}
		staticFS, err := fs.Sub(staticFiles, "static")
		if err != nil {
			log.Fatalf("embed: %v", err)
		}
		if err := server.New(addr, staticFS).Run(); err != nil {
			log.Fatalf("сервер: %v", err)
		}
		return
	}

	// CLI-режим: lyric-video-factory template.json [N]
	templatePath := defaultTemplate
	n := 1

	if len(os.Args) > 1 {
		templatePath = os.Args[1]
	}
	if len(os.Args) > 2 {
		var err error
		n, err = strconv.Atoi(os.Args[2])
		if err != nil || n <= 0 {
			log.Fatalf("N должен быть положительным целым числом, получено %q", os.Args[2])
		}
	}

	t, err := tmpl.Parse(templatePath)
	if err != nil {
		log.Fatalf("шаблон: %v", err)
	}
	if t.Audio == "" {
		t.Audio = tmpl.FindAudio(inputDir)
	}

	pool, err := render.LoadPool(inputDir)
	if err != nil {
		log.Fatalf("пул: %v", err)
	}

	log.Printf("шаблон: %d текстов, %d видео-сегментов", len(t.Texts), len(t.VideoSegments))
	log.Printf("пул:    %d клипов", len(pool.Clips))
	log.Printf("задача: %d вариант(ов) → %s/", n, outputDir)

	if err := render.Run(&render.Job{
		Template:  t,
		Pool:      pool,
		N:         n,
		OutputDir: outputDir,
	}); err != nil {
		log.Fatalf("ошибка: %v", err)
	}

	absPath, _ := filepath.Abs(outputDir)
	fmt.Printf("\nГотово! Результаты в: %s\n", absPath)
}

func checkBinaries() {
	for _, bin := range []string{"ffmpeg", "ffprobe"} {
		if _, err := exec.LookPath(bin); err != nil {
			log.Fatalf("%s не найден в PATH", bin)
		}
	}
}
