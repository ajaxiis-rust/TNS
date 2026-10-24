package langpack

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// Lemmatizer — часть пакета «лемматизатор» (D-11), зафиксированная в манифесте.
type Lemmatizer struct {
	Kind     string `json:"kind"`
	Protocol string `json:"protocol"`
}

// Manifest — манифест языкового пакета (T-01-08, шаг 1).
type Manifest struct {
	ID         string            `json:"id"`
	Name       string            `json:"name"`
	Lang       string            `json:"lang"`
	Version    string            `json:"version"`
	Lemmatizer Lemmatizer        `json:"lemmatizer"`
	Files      map[string]string `json:"files"`
}

// BrokenFilesError — расхождение пакета: перечень битых файлов (шаг 2).
type BrokenFilesError struct {
	Files []string
}

func (e *BrokenFilesError) Error() string {
	return fmt.Sprintf("langpack: broken files: %v", e.Files)
}

// LoadManifest читает манифест из path.
func LoadManifest(path string) (*Manifest, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var m Manifest
	if err := json.NewDecoder(f).Decode(&m); err != nil {
		return nil, err
	}
	return &m, nil
}

// verifyFiles сверяет наличие и суммы файлов манифеста относительно baseDir.
// Расхождение — *BrokenFilesError с перечнем битых файлов.
func (m *Manifest) verifyFiles(baseDir string) error {
	var broken []string
	for name, want := range m.Files {
		path := filepath.Join(baseDir, filepath.FromSlash(name))
		sum, err := sha256File(path)
		if err != nil || sum != want {
			broken = append(broken, name)
		}
	}
	if len(broken) > 0 {
		return &BrokenFilesError{Files: broken}
	}
	return nil
}

func sha256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
