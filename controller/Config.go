package controller

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/joho/godotenv"
)

// Command define un comando y su acción
type Command struct {
	TriggerWords []string       `json:"trigger_words"`
	Action       string         `json:"action"`
	Description  string         `json:"description"`
	Parameters   map[string]any `json:"parameters,omitempty"`
}

// ConfigData representa la estructura del archivo config.json
type ConfigData struct {
	YouTube struct {
		DefaultQuery string            `json:"default_query"`
		XPath        map[string]string `json:"xpath"`
	} `json:"youtube"`
	BrowserUserDirectory string `json:"browser_user_directory"`
}

// Config configuración del sistema
type Config struct {
	Headless        string
	ChromePath      string
	CookiesBasePath string
	ConfigJSON      string

	// Cache de configuración JSON (Thread-safe)
	configData *ConfigData
	configMu   sync.Mutex

	Log           *Log
	commandsCache map[string]Command
}

// NewConfig crea una nueva instancia de Config
func NewConfig() *Config {
	envPath := ".env"
	if _, err := os.Stat(envPath); err == nil {
		if err := godotenv.Load(); err != nil {
			fmt.Printf("⚠️  Advertencia: No se pudo cargar .env: %v\n", err)
		}
	}

	c := &Config{
		Headless:        getEnvDefault("HEADLESS", "true"),
		ChromePath:      getEnvDefault("CHROME_PATH", ""),
		CookiesBasePath: getEnvDefault("COOKIES_PATH", "./cookies"),
		ConfigJSON:      getEnvDefault("CONFIG_JSON", "./config.json"),
		Log:             NewLog(),
	}

	// Cargar configuración JSON al inicio
	c.loadConfigJSON()

	return c
}

// loadConfigJSON carga y parsea el archivo config.json de forma segura y única
func (c *Config) loadConfigJSON() {
	c.configMu.Lock()
	defer c.configMu.Unlock()

	if c.configData != nil {
		return // Ya está cargado en caché
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

// GetUserBrowserDirectory retorna la ruta del directorio de usuario del navegador.
// ⚠️ Carga ÚNICAMENTE desde config.json. No lee .env.
func (c *Config) GetUserBrowserDirectory() string {
	c.loadConfigJSON()

	if c.configData != nil && strings.TrimSpace(c.configData.BrowserUserDirectory) != "" {
		return expandUser(c.configData.BrowserUserDirectory)
	}

	// Fallback de seguridad mínimo solo para evitar crashes si el JSON está vacío,
	// pero NUNCA lee el .env.
	return expandUser("~/.config/BraveSoftware/Brave-Browser-Playwright")
}

// GetYouTubeQuery retorna la query por defecto para YouTube.
// ⚠️ Carga ÚNICAMENTE desde config.json. No lee .env.
func (c *Config) GetYouTubeQuery() string {
	c.loadConfigJSON()

	if c.configData != nil && strings.TrimSpace(c.configData.YouTube.DefaultQuery) != "" {
		return c.configData.YouTube.DefaultQuery
	}

	return "" // Retorna vacío si no está en el JSON
}

// GetYouTubeXPath retorna el diccionario de XPaths para YouTube.
// ⚠️ Carga ÚNICAMENTE desde config.json. No lee .env.
func (c *Config) GetYouTubeXPath() map[string]string {
	c.loadConfigJSON()

	if c.configData != nil && len(c.configData.YouTube.XPath) > 0 {
		return c.configData.YouTube.XPath
	}

	// Retorna mapa vacío si no está definido en el JSON
	return make(map[string]string)
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
