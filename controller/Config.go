// controller/Config.go
package controller

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/joho/godotenv"
)

// Command define un comando y su acción
type Command struct {
	TriggerWords []string       `json:"trigger_words"`
	Action       string         `json:"action"`
	Description  string         `json:"description"`
	Parameters   map[string]any `json:"parameters,omitempty"`
}

// Config configuración del sistema
type Config struct {
	Headless        string
	ChromePath      string
	CookiesBasePath string
	ConfigJSON      string
	UserBrowserDir  string
	Log             *Log
	commandsCache   map[string]Command
}

// NewConfig crea una nueva instancia de Config
func NewConfig() *Config {
	envPath := ".env"
	if _, err := os.Stat(envPath); err == nil {
		if err := godotenv.Load(); err != nil {
			fmt.Printf("⚠️  Advertencia: No se pudo cargar .env: %v\n", err)
		}
	}

	return &Config{
		Headless:        getEnvDefault("HEADLESS", "true"),
		ChromePath:      getEnvDefault("CHROME_PATH", ""),
		CookiesBasePath: getEnvDefault("COOKIES_PATH", "./cookies"),
		ConfigJSON:      getEnvDefault("CONFIG_JSON", "./config.json"),
		UserBrowserDir:  getEnvDefault("USER_BROWSER_DIR", "~/.config/BraveSoftware/Brave-Browser-Playwright"),
		Log:             NewLog(),
	}
}

// GetChromePaths retorna lista de paths válidos para Chrome/Chromium
func (c *Config) GetChromePaths() []string {
	var paths []string

	// 1️⃣ Si se especificó en .env y es ejecutable, usar ese primero
	if c.ChromePath != "" && isExecutable(c.ChromePath) {
		paths = append(paths, c.ChromePath)
	}

	// 2️⃣ Fallback: buscar en rutas comunes del SO
	paths = append(paths, findChromePaths()...)

	return paths
}

// GetChromePath retorna el primer path válido de Chrome o ""
func (c *Config) GetChromePath() string {
	paths := c.GetChromePaths()
	if len(paths) > 0 {
		return paths[0]
	}
	return ""
}

// GetCookiesPath retorna la ruta del archivo de storage state para Playwright
func (c *Config) GetCookiesPath() string {
	return filepath.Join(c.CookiesBasePath, "youtube_state.json")
}

// CookiesExist verifica si ya existe un estado de sesión guardado
func (c *Config) CookiesExist() bool {
	_, err := os.Stat(c.GetCookiesPath())
	return err == nil
}

// ClearCookies elimina el archivo de cookies para forzar sesión limpia
func (c *Config) ClearCookies() {
	cookiesPath := c.GetCookiesPath()
	if _, err := os.Stat(cookiesPath); err == nil {
		_ = os.Remove(cookiesPath)
	}
}

// GetUserBrowserDirectory retorna la ruta del directorio de usuario del navegador
func (c *Config) GetUserBrowserDirectory() string {
	if c.UserBrowserDir == "" {
		return expandUser("~/.config/BraveSoftware/Brave-Browser-Playwright")
	}
	return expandUser(c.UserBrowserDir)
}

// GetCommands retorna los comandos desde config.json o usa los defaults
func (c *Config) GetCommands() map[string]Command {
	if c.commandsCache != nil {
		return c.commandsCache
	}

	defaultCommands := c.getDefaultCommands()

	// Intentar cargar desde config.json
	if data, err := os.ReadFile(c.ConfigJSON); err == nil {
		var jsonData map[string]json.RawMessage
		if err := json.Unmarshal(data, &jsonData); err == nil {
			if commandsRaw, ok := jsonData["commands"]; ok {
				var loadedCommands map[string]Command
				if err := json.Unmarshal(commandsRaw, &loadedCommands); err == nil {
					c.commandsCache = loadedCommands
					c.Log.Comentario("INFO", fmt.Sprintf("✅ Cargados %d comandos", len(c.commandsCache)))
					return c.commandsCache
				}
			}
		}
	}

	c.commandsCache = defaultCommands
	c.Log.Comentario("INFO", "📝 Usando comandos por defecto")
	return c.commandsCache
}

// getDefaultCommands comandos por defecto del sistema (fallback)
func (c *Config) getDefaultCommands() map[string]Command {
	return map[string]Command{
		"saluda": {
			TriggerWords: []string{"saluda", "saludar", "hola", "dime hola"},
			Action:       "/saluda",
			Description:  "Saluda al bot",
		},
		"reproduce": {
			TriggerWords: []string{"reproduce", "pon", "toca", "play", "reproducir", "pon música", "buscar", "search", "encuentra", "busca"},
			Action:       "/play",
			Description:  "Reproduce música (ej: 'reproduce lofi')",
			Parameters:   map[string]any{"type": "query"},
		},
		"pausa": {
			TriggerWords: []string{"pausa", "pausar", "detén", "para", "detener", "espera"},
			Action:       "/pause",
			Description:  "Pausa la reproducción",
		},
		"continuar": {
			TriggerWords: []string{"continuar", "reanudar", "sigue", "resume", "despausa"},
			Action:       "/resume",
			Description:  "Reanuda la reproducción",
		},
		"siguiente": {
			TriggerWords: []string{"siguiente", "next", "siguiente video", "salta"},
			Action:       "/next",
			Description:  "Reproduce el siguiente video",
		},
		"anterior": {
			TriggerWords: []string{"anterior", "prev", "anterior video", "atrás", "retrocede"},
			Action:       "/prev",
			Description:  "Reproduce el video anterior",
		},
		"pantalla completa": {
			TriggerWords: []string{"pantalla completa", "full screen", "fullscreen", "ampliar"},
			Action:       "/fullscreen",
			Description:  "Activa pantalla completa",
		},
		"volumen": {
			TriggerWords: []string{"volumen", "sube volumen", "baja volumen", "volumen a", "set volume"},
			Action:       "/volume",
			Description:  "Ajusta volumen (ej: 'volumen 50')",
			Parameters:   map[string]any{"type": "integer", "min": 0, "max": 100},
		},
		"ayuda": {
			TriggerWords: []string{"ayuda", "comandos", "qué puedes hacer", "help", "que hacer"},
			Action:       "/help",
			Description:  "Muestra esta ayuda",
		},
		"apaga": {
			TriggerWords: []string{"apagar", "salir", "adios", "apagate", "cierra todo", "basta", "exit", "terminar", "adiós", "hasta luego"},
			Action:       "/shutdown",
			Description:  "Apaga el ordenador",
		},
	}
}

// ──────────────────────────────────────────────
// Helpers
// ──────────────────────────────────────────────

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

// isExecInPath verifica si un ejecutable está en el PATH
func isExecInPath(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}
