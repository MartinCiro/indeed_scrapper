package controller

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

func getEnvDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func isExecutable(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	if runtime.GOOS == "windows" {
		ext := strings.ToLower(filepath.Ext(path))
		return ext == ".exe" || ext == ".bat" || ext == ".cmd"
	}
	return !info.IsDir() && info.Mode()&0111 != 0
}

func findChromePaths() []string {
	var paths []string
	var candidates []string

	switch runtime.GOOS {
	case "linux":
		candidates = []string{
			"/usr/bin/brave-browser",
			"/usr/bin/brave-browser-stable",
			"/usr/bin/google-chrome",
			"/usr/bin/google-chrome-stable",
			"/usr/bin/chromium",
			"/usr/bin/chromium-browser",
			"/snap/bin/chromium",
			"/opt/google/chrome/chrome",
		}
	case "darwin":
		candidates = []string{
			"/Applications/Brave Browser.app/Contents/MacOS/Brave Browser",
			"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
			"/Applications/Chromium.app/Contents/MacOS/Chromium",
		}
	case "windows":
		candidates = []string{
			`C:\Program Files\BraveSoftware\Brave-Browser\Application\brave.exe`,
			`C:\Program Files (x86)\BraveSoftware\Brave-Browser\Application\brave.exe`,
			`C:\Program Files\Google\Chrome\Application\chrome.exe`,
			`C:\Program Files (x86)\Google\Chrome\Application\chrome.exe`,
		}
	}

	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			paths = append(paths, p)
		}
	}

	return paths
}

func expandUser(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return filepath.Join(home, path[2:])
	}
	return path
}

func isExecInPath(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

// loadConfigJSON carga y parsea el archivo config.json de forma segura y única
func (c *Config) loadConfigJSON() {
	c.configMu.Lock()
	defer c.configMu.Unlock()

	if c.configData != nil {
		return
	}

	data, err := os.ReadFile(c.ConfigJSON)
	if err != nil {
		c.Log.Comentario("WARNING", fmt.Sprintf("No se pudo leer %s: %v.", c.ConfigJSON, err))
		c.configData = &ConfigData{}
		return
	}

	var parsed ConfigData
	if err := json.Unmarshal(data, &parsed); err != nil {
		c.Log.Comentario("WARNING", fmt.Sprintf("Error parseando %s: %v.", c.ConfigJSON, err))
		c.configData = &ConfigData{}
	} else {
		c.configData = &parsed
		c.Log.Comentario("INFO", "✅ Configuración cargada estrictamente desde config.json")
	}
}

// ExtractUsername obtiene el nombre de usuario del correo para usarlo en la ruta
// Ej: "a@gmail.com" -> "a", "b.gmail.com" -> "b"
func ExtractUsername(email string) string {
	parts := strings.Split(email, "@")
	username := parts[0]

	// Fallback si no tiene '@' (ej: "b.gmail.com")
	if username == email {
		partsDot := strings.Split(email, ".")
		username = partsDot[0]
	}

	// Reemplazar puntos por guiones bajos para nombres de carpeta seguros
	return strings.ReplaceAll(username, ".", "_")
}
