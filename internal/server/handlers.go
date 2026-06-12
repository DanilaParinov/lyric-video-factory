package server

import (
	"archive/zip"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"lyric-video-factory/internal/render"
	"lyric-video-factory/internal/tmpl"
)

// --- POST /api/shutdown ---

func (s *Server) handleShutdown(w http.ResponseWriter, r *http.Request) {
	jsonResp(w, http.StatusOK, map[string]string{"status": "ok"})
	go func() {
		time.Sleep(150 * time.Millisecond)
		cleanupDir(filepath.Join(uploadDir, "audio"))
		os.Exit(0)
	}()
}

func cleanupDir(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if !e.IsDir() {
			os.Remove(filepath.Join(dir, e.Name()))
		}
	}
}

// --- POST /api/upload ---

func (s *Server) handleUpload(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		jsonErr(w, "не могу прочитать форму: "+err.Error(), http.StatusBadRequest)
		return
	}

	fileType := r.FormValue("type")
	destDir := uploadSubdir(fileType)
	if destDir == "" {
		jsonErr(w, "неверный type: ожидается video|audio|template|font", http.StatusBadRequest)
		return
	}

	f, fh, err := r.FormFile("file")
	if err != nil {
		jsonErr(w, "нет поля file: "+err.Error(), http.StatusBadRequest)
		return
	}
	defer f.Close()

	dstPath := filepath.Join(destDir, filepath.Base(fh.Filename))
	dst, err := os.Create(dstPath)
	if err != nil {
		jsonErr(w, "создаю файл: "+err.Error(), http.StatusInternalServerError)
		return
	}

	if _, err := io.Copy(dst, f); err != nil {
		dst.Close()
		jsonErr(w, "записываю файл: "+err.Error(), http.StatusInternalServerError)
		return
	}
	dst.Close() // закрываем до вызова ffprobe

	// Видеоклипы сразу добавляем в пул
	if fileType == "video" {
		dur, _ := render.ProbeDuration(dstPath)
		s.pool.Add(render.Clip{Path: dstPath, Duration: dur})
	}

	jsonResp(w, http.StatusOK, map[string]string{"path": dstPath, "name": fh.Filename})
}

func uploadSubdir(fileType string) string {
	switch fileType {
	case "video":
		return filepath.Join(uploadDir, "video")
	case "audio":
		return filepath.Join(uploadDir, "audio")
	case "template":
		return filepath.Join(uploadDir, "template")
	case "font":
		return filepath.Join(uploadDir, "font")
	default:
		return ""
	}
}

// --- GET /api/pool ---

type poolEntry struct {
	Name     string  `json:"name"`
	Path     string  `json:"path"`
	Duration float64 `json:"duration"`
}

func (s *Server) handlePool(w http.ResponseWriter, r *http.Request) {
	entries := s.pool.Entries()
	clips := make([]poolEntry, len(entries))
	for i, c := range entries {
		clips[i] = poolEntry{
			Name:     filepath.Base(c.Path),
			Path:     c.Path,
			Duration: c.Duration,
		}
	}
	jsonResp(w, http.StatusOK, clips)
}

// --- DELETE /api/pool ---

func (s *Server) handleClearPool(w http.ResponseWriter, r *http.Request) {
	n := s.pool.Clear()
	jsonResp(w, http.StatusOK, map[string]int{"deleted": n})
}

// --- DELETE /api/pool/clip ---

func (s *Server) handleDeleteClip(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	if name == "" || strings.ContainsAny(name, `/\`) || strings.Contains(name, "..") {
		jsonErr(w, "недопустимое имя файла", http.StatusBadRequest)
		return
	}
	if !s.pool.Remove(name) {
		jsonErr(w, "клип не найден в пуле", http.StatusNotFound)
		return
	}
	jsonResp(w, http.StatusOK, map[string]string{"removed": name})
}

// --- GET /api/template  (загрузить по пути на диске) ---
// --- POST /api/template (распарсить сырой JSON из тела) ---

func (s *Server) handleGetTemplate(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if path == "" {
		jsonErr(w, "параметр path обязателен", http.StatusBadRequest)
		return
	}
	if strings.Contains(path, "..") {
		jsonErr(w, "недопустимый путь", http.StatusBadRequest)
		return
	}
	t, err := tmpl.Parse(path)
	if err != nil {
		jsonErr(w, err.Error(), http.StatusBadRequest)
		return
	}
	resolveAudio(t)
	jsonResp(w, http.StatusOK, t)
}

func (s *Server) handleParseTemplate(w http.ResponseWriter, r *http.Request) {
	data, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		jsonErr(w, "читаю тело: "+err.Error(), http.StatusBadRequest)
		return
	}
	t, err := tmpl.ParseData(data)
	if err != nil {
		jsonErr(w, err.Error(), http.StatusBadRequest)
		return
	}
	resolveAudio(t)
	jsonResp(w, http.StatusOK, t)
}

// resolveAudio проверяет, что аудиофайл из шаблона существует на диске.
// Если нет — пробует найти аудио автоматически; если и там ничего, оставляет пустым.
func resolveAudio(t *tmpl.Template) {
	if t.Audio != "" {
		if _, err := os.Stat(t.Audio); err != nil {
			t.Audio = "" // файл не найден — сбрасываем, пробуем autodiscover
		}
	}
	if t.Audio == "" {
		t.Audio = tmpl.FindAudio(filepath.Join(uploadDir, "audio"), inputDir)
	}
}

// --- POST /api/jobs ---

type createJobReq struct {
	Template     string         `json:"template"`      // путь к файлу; либо template_data
	TemplateData *tmpl.Template `json:"template_data"` // инлайн-шаблон (приоритет над template)
	N            int            `json:"n"`
	DimLevel     float64        `json:"dim_level"`
}

func (s *Server) handleCreateJob(w http.ResponseWriter, r *http.Request) {
	var req createJobReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonErr(w, "неверный JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.TemplateData == nil && req.Template == "" {
		jsonErr(w, "нужно поле template или template_data", http.StatusBadRequest)
		return
	}
	if req.N <= 0 {
		req.N = 1
	}

	job := s.jobs.create(req.Template, req.TemplateData, req.N, req.DimLevel)
	go s.runJob(job)
	jsonResp(w, http.StatusAccepted, job.Snapshot())
}

func (s *Server) handleListJobs(w http.ResponseWriter, r *http.Request) {
	jsonResp(w, http.StatusOK, s.jobs.list())
}

func (s *Server) handleGetJob(w http.ResponseWriter, r *http.Request) {
	job, ok := s.jobs.get(r.PathValue("id"))
	if !ok {
		jsonErr(w, "задача не найдена", http.StatusNotFound)
		return
	}
	jsonResp(w, http.StatusOK, job.Snapshot())
}

// --- GET /api/jobs/{id}/download ---

func (s *Server) handleDownloadAll(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	job, ok := s.jobs.get(id)
	if !ok {
		jsonErr(w, "задача не найдена", http.StatusNotFound)
		return
	}
	snap := job.Snapshot()
	if snap.Status != StatusDone {
		jsonErr(w, "задача ещё не завершена", http.StatusConflict)
		return
	}
	if len(snap.Results) == 0 {
		jsonErr(w, "нет файлов для скачивания", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Disposition", `attachment; filename="results.zip"`)
	w.Header().Set("Content-Type", "application/zip")

	zw := zip.NewWriter(w)
	for _, name := range snap.Results {
		f, err := os.Open(filepath.Join(outputDir, id, name))
		if err != nil {
			continue
		}
		fw, err := zw.Create(name)
		if err != nil {
			f.Close()
			continue
		}
		io.Copy(fw, f)
		f.Close()
	}
	zw.Close()
}

// --- GET /api/jobs/{id}/results/{file} ---

func (s *Server) handleDownloadResult(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	file := r.PathValue("file")

	if strings.ContainsAny(file, `/\`) || file == ".." {
		jsonErr(w, "недопустимое имя файла", http.StatusBadRequest)
		return
	}
	job, ok := s.jobs.get(id)
	if !ok {
		jsonErr(w, "задача не найдена", http.StatusNotFound)
		return
	}
	if v := job.Snapshot(); v.Status != StatusDone {
		jsonErr(w, "задача ещё не завершена (status: "+string(v.Status)+")", http.StatusConflict)
		return
	}
	http.ServeFile(w, r, filepath.Join(outputDir, id, file))
}

// --- фоновое выполнение ---

func (s *Server) runJob(job *Job) {
	job.setRunning()
	log.Printf("job %s: запуск %d вариант(ов)", job.ID, job.N)

	var t *tmpl.Template
	if job.TemplateData != nil {
		t = job.TemplateData
		tmpl.ApplyDefaults(t)
		if err := t.Validate(); err != nil {
			job.setError("валидация шаблона: " + err.Error())
			log.Printf("job %s: ошибка — %v", job.ID, err)
			return
		}
	} else {
		var err error
		t, err = tmpl.Parse(job.Template)
		if err != nil {
			job.setError("шаблон: " + err.Error())
			log.Printf("job %s: ошибка — %v", job.ID, err)
			return
		}
	}
	t.DimLevel = job.DimLevel
	if t.Audio == "" {
		t.Audio = tmpl.FindAudio(filepath.Join(uploadDir, "audio"), inputDir)
	}

	pool := s.pool.AsPool()
	if len(pool.Clips) == 0 {
		job.setError("пул клипов пуст")
		log.Printf("job %s: ошибка — пул пуст", job.ID)
		return
	}

	outDir := filepath.Join(outputDir, job.ID)
	if err := render.Run(&render.Job{
		Template:  t,
		Pool:      pool,
		N:         job.N,
		OutputDir: outDir,
	}); err != nil {
		job.setError(err.Error())
		log.Printf("job %s: ошибка — %v", job.ID, err)
		return
	}

	entries, _ := os.ReadDir(outDir)
	var results []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".mp4") {
			results = append(results, e.Name())
		}
	}
	job.setDone(results)
	log.Printf("job %s: готово, %d файл(ов)", job.ID, len(results))
	s.cleanupUploadedClips()
}

// cleanupUploadedClips удаляет с диска и из пула видеофайлы из uploads/video.
// Файлы из input/ не трогаются — они являются постоянной библиотекой.
func (s *Server) cleanupUploadedClips() {
	videoUploadDir := filepath.Clean(filepath.Join(uploadDir, "video"))
	for _, clip := range s.pool.Entries() {
		if filepath.Dir(filepath.Clean(clip.Path)) != videoUploadDir {
			continue
		}
		os.Remove(clip.Path)
		s.pool.Remove(filepath.Base(clip.Path))
		log.Printf("удалён загруженный клип: %s", filepath.Base(clip.Path))
	}
}

// --- helpers ---

func jsonResp(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func jsonErr(w http.ResponseWriter, msg string, status int) {
	jsonResp(w, status, map[string]string{"error": msg})
}
