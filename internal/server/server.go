package server

import (
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"
)

const (
	inputDir  = "input"
	uploadDir = "uploads"
	outputDir = "output"
	staticDir = "static"
)

type Server struct {
	addr     string
	jobs     *JobStore
	pool     *PoolManager
	staticFS fs.FS
}

func New(addr string, staticFS fs.FS) *Server {
	return &Server{addr: addr, jobs: newJobStore(), staticFS: staticFS}
}

func (s *Server) Run() error {
	for _, dir := range []string{
		uploadDir + "/video",
		uploadDir + "/audio",
		uploadDir + "/template",
		uploadDir + "/font",
		outputDir,
	} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("создаю %s: %w", dir, err)
		}
	}

	// Инициализируем пул из обеих директорий
	s.pool = newPoolManager(inputDir, filepath.Join(uploadDir, "video"))
	log.Printf("пул: %d клип(ов) загружено", s.pool.Len())

	mux := http.NewServeMux()

	mux.HandleFunc("POST /api/upload", s.handleUpload)
	mux.HandleFunc("GET /api/template", s.handleGetTemplate)
	mux.HandleFunc("POST /api/template", s.handleParseTemplate)
	mux.HandleFunc("GET /api/pool", s.handlePool)
	mux.HandleFunc("DELETE /api/pool", s.handleClearPool)
	mux.HandleFunc("DELETE /api/pool/clip", s.handleDeleteClip)
	mux.HandleFunc("POST /api/jobs", s.handleCreateJob)
	mux.HandleFunc("GET /api/jobs", s.handleListJobs)
	mux.HandleFunc("GET /api/jobs/{id}", s.handleGetJob)
	mux.HandleFunc("GET /api/jobs/{id}/results/{file}", s.handleDownloadResult)
	mux.HandleFunc("POST /api/shutdown", s.handleShutdown)

	mux.Handle("/", http.FileServerFS(s.staticFS))

	url := listenURL(s.addr)
	log.Printf("сервер запущен: %s", url)

	go func() {
		time.Sleep(300 * time.Millisecond)
		openBrowser(url)
	}()

	return http.ListenAndServe(s.addr, mux)
}

func listenURL(addr string) string {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return "http://localhost:8080"
	}
	if host == "" || host == "0.0.0.0" {
		host = "localhost"
	}
	return "http://" + host + ":" + port
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	if err := cmd.Start(); err != nil {
		log.Printf("не могу открыть браузер: %v", err)
	}
}
