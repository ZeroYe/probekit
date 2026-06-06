package mcp

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
)

const uploadDir = "uploads"

func (s *Server) handleUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := r.ParseMultipartForm(32 << 20); err != nil {
		http.Error(w, fmt.Sprintf("parse form: %s", err), http.StatusBadRequest)
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, fmt.Sprintf("missing file field: %s", err), http.StatusBadRequest)
		return
	}
	defer file.Close()

	dir := filepath.Join(s.deps.ConfigDir, uploadDir)
	if err := os.MkdirAll(dir, 0755); err != nil {
		http.Error(w, fmt.Sprintf("create upload dir: %s", err), http.StatusInternalServerError)
		return
	}

	savedName := uniqueName(header.Filename)
	savedPath := filepath.Join(dir, savedName)

	dst, err := os.Create(savedPath)
	if err != nil {
		http.Error(w, fmt.Sprintf("create file: %s", err), http.StatusInternalServerError)
		return
	}
	defer dst.Close()

	if _, err := io.Copy(dst, file); err != nil {
		os.Remove(savedPath)
		http.Error(w, fmt.Sprintf("write file: %s", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(savedPath))
}

func uniqueName(original string) string {
	b := make([]byte, 8)
	rand.Read(b)
	return fmt.Sprintf("%s_%s", hex.EncodeToString(b), filepath.Base(original))
}
