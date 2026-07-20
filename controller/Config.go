package controller

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/joho/godotenv"
)

// ConfigData representa la estructura del archivo config.json
type ConfigData struct {
	Indeed               IndeedConfig `json:"indeed"`
	Users                []string     `json:"users"`
	BrowserUserDirectory string       `json:"browser_user_directory"`
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

	Log *Log
}

// IndeedSearch representa los selectores para la búsqueda de empleos
type IndeedSearch struct {
	InputWhat  string `json:"input_what"`
	InputWhere string `json:"input_where"`
	BtnSearch  string `json:"btn_search"`
}

// IndeedConfig representa la configuración del portal Indeed
type IndeedConfig struct {
	URL    string            `json:"url"`
	XPath  map[string]string `json:"xpath"`
	Search IndeedSearch      `json:"search"`
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
func (c *Config) GetCookiesPath(username string) string {
	return filepath.Join("cookies", "users", username, "indeed.json")
}

// CookiesExist verifica si ya existe un estado de sesión guardado
func (c *Config) CookiesExist(username string) bool {
	_, err := os.Stat(c.GetCookiesPath(username))
	return err == nil
}

// ClearCookies elimina el archivo de cookies para forzar sesión limpia
func (c *Config) ClearCookies(username string) {
	cookiesPath := c.GetCookiesPath(username)
	if _, err := os.Stat(cookiesPath); err == nil {
		_ = os.Remove(cookiesPath)
	}
}

// GetIndeedConfig retorna la configuración específica del portal Indeed
func (c *Config) GetIndeedConfig() IndeedConfig {
	c.loadConfigJSON()

	if c.configData != nil && c.configData.Indeed.URL != "" {
		return c.configData.Indeed
	}

	// Fallback por defecto
	return IndeedConfig{
		URL: "https://co.indeed.com",
		XPath: map[string]string{
			"btn_ingresar_login": "(//a[contains(text(), 'Ingresar')])[1]",
			"input_correo":       "//input[@name='__email']",
			"btn_continuar":      "(//button[@type='submit' and contains(normalize-space(), 'Continuar')])[1]",
			"btn_iniciar_codigo": "//a[contains(translate(., 'INICIARSESIOÓN', 'iniciarsesioón'), 'iniciar')]",
			"btn_ingresar_code":  "//label[contains(translate(., 'INGRESARCÓODIGO', 'ingresarcóodigo'), 'código')]",
			"input_code":         "//label[contains(translate(., 'INGRESARCÓODIGO', 'ingresarcóodigo'), 'código')]/following-sibling::span/input",
			"btn_ingresar":       "//a[contains(., 'No tienes')]/parent::div/preceding-sibling::button",
			"btn_ahora_no":       "//a[contains(translate(., 'AHORA', 'ahora'), 'ahora')]",
		},
		Search: IndeedSearch{
			InputWhat:  "//input[@name='q']",
			InputWhere: "//input[@name='l']",
			BtnSearch:  "//button[@type='submit' and contains(normalize-space(), 'Buscar')]",
		},
	}
}

// GetIndeedUsers retorna la lista de usuarios configurados para el login
func (c *Config) GetIndeedUsers() []string {
	c.loadConfigJSON()
	if c.configData != nil && len(c.configData.Users) > 0 {
		return c.configData.Users
	}
	return []string{}
}
