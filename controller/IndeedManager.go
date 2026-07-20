package controller

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mxschmitt/playwright-go"
)

type IndeedManager struct {
	page      playwright.Page
	config    *Config
	indeedCfg IndeedConfig
}

func NewIndeedManager(page playwright.Page, config *Config) *IndeedManager {
	return &IndeedManager{
		page:      page,
		config:    config,
		indeedCfg: config.GetIndeedConfig(),
	}
}

// Login realiza el flujo de autenticación lineal y guarda las cookies para un usuario específico
func (im *IndeedManager) Login(email, username string) error {
	im.config.Log.Comentario("INFO", fmt.Sprintf("🔐 Iniciando login en Indeed para: %s", email))

	// 1️⃣ Navegar al portal
	if _, err := im.page.Goto(im.indeedCfg.URL, playwright.PageGotoOptions{
		WaitUntil: playwright.WaitUntilStateNetworkidle,
	}); err != nil {
		return fmt.Errorf("error navegando a Indeed: %w", err)
	}

	// 2️⃣ Hacer clic en "Ingresar"
	btnLogin := im.page.Locator(im.indeedCfg.XPath["btn_ingresar_login"])
	if err := btnLogin.First().Click(); err != nil {
		return fmt.Errorf("error haciendo clic en 'Ingresar': %w", err)
	}

	// 3️⃣ Esperar y llenar el correo
	im.page.WaitForSelector(im.indeedCfg.XPath["input_correo"], playwright.PageWaitForSelectorOptions{
		State: playwright.WaitForSelectorStateVisible,
	})
	if err := im.page.Locator(im.indeedCfg.XPath["input_correo"]).Fill(email); err != nil {
		return fmt.Errorf("error llenando email: %w", err)
	}

	// 4️⃣ Hacer clic en "Continuar"
	if err := im.page.Locator(im.indeedCfg.XPath["btn_continuar"]).First().Click(); err != nil {
		return fmt.Errorf("error haciendo clic en 'Continuar': %w", err)
	}

	// 5️⃣ Esperar y presionar "Iniciar sesión con código" (btn_iniciar_codigo)
	im.config.Log.Comentario("INFO", "⏳ Esperando botón de inicio de sesión con código...")
	im.page.WaitForSelector(im.indeedCfg.XPath["btn_iniciar_codigo"], playwright.PageWaitForSelectorOptions{
		State: playwright.WaitForSelectorStateVisible,
	})
	if err := im.page.Locator(im.indeedCfg.XPath["btn_iniciar_codigo"]).First().Click(); err != nil {
		return fmt.Errorf("error haciendo clic en 'Iniciar con código': %w", err)
	}

	// 6️⃣ Esperar input de código e ingresar
	im.config.Log.Comentario("INFO", "⏳ Esperando input de código...")
	im.page.WaitForSelector(im.indeedCfg.XPath["input_code"], playwright.PageWaitForSelectorOptions{
		State: playwright.WaitForSelectorStateVisible,
	})

	// 6️⃣ OBTENER CÓDIGO AUTOMÁTICAMENTE DESDE GMAIL
	oauthService := NewOAuthService(im.config)
	code, err := oauthService.GetCodeFromGmail(username)
	if err != nil {
		return fmt.Errorf("error obteniendo código 2FA desde Gmail: %w", err)
	}

	if err := im.page.Locator(im.indeedCfg.XPath["input_code"]).Fill(code); err != nil {
		return fmt.Errorf("error llenando código: %w", err)
	}

	// 7️⃣ Hacer clic en "Ingresar" (btn_ingresar) para enviar el código
	im.config.Log.Comentario("INFO", "✅ Código ingresado, enviando formulario...")
	if err := im.page.Locator(im.indeedCfg.XPath["btn_ingresar"]).First().Click(); err != nil {
		return fmt.Errorf("error haciendo clic en 'Ingresar' (submit código): %w", err)
	}

	// 8️⃣ Esperar a que la sesión se establezca
	im.page.WaitForTimeout(3000)
	im.config.Log.Comentario("SUCCESS", "✅ Login completado exitosamente")

	// 9️⃣ Guardar cookies filtradas
	return im.SaveCookies(username)
}

// SaveCookies guarda el estado en <username>/cookie/indeed.json
// SaveCookies guarda el estado de la sesión FILTRANDO solo las cookies y localStorage de Indeed
func (im *IndeedManager) SaveCookies(username string) error {
	cookiesPath := im.config.GetCookiesPath(username)

	// 1. Crear directorios anidados si no existen (cookies/users/<nom_user>/)
	dir := filepath.Dir(cookiesPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("error creando directorio %s: %w", dir, err)
	}

	context := im.page.Context()

	// 2. Obtener el estado completo usando el método oficial de Playwright
	state, err := context.StorageState()
	if err != nil {
		return fmt.Errorf("error obteniendo estado de almacenamiento: %w", err)
	}

	// 3. Convertir el estado a JSON y luego a un mapa genérico.
	// Esto nos permite filtrar sin depender de los nombres de tipos internos de playwright-go
	stateBytes, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("error serializando estado original: %w", err)
	}

	var genericState map[string]any
	if err := json.Unmarshal(stateBytes, &genericState); err != nil {
		return fmt.Errorf("error parseando estado a mapa genérico: %w", err)
	}

	// 4. Filtrar COOKIES: mantener solo las que contengan "indeed" en el dominio
	if cookies, ok := genericState["cookies"].([]any); ok {
		var filteredCookies []any
		for _, c := range cookies {
			if cookieMap, ok := c.(map[string]any); ok {
				if domain, ok := cookieMap["domain"].(string); ok {
					if strings.Contains(strings.ToLower(domain), "indeed") {
						filteredCookies = append(filteredCookies, c)
					}
				}
			}
		}
		genericState["cookies"] = filteredCookies
	}

	// 5. Filtrar ORIGINS (localStorage): mantener solo los de Indeed
	if origins, ok := genericState["origins"].([]any); ok {
		var filteredOrigins []any
		for _, o := range origins {
			if originMap, ok := o.(map[string]any); ok {
				if originStr, ok := originMap["origin"].(string); ok {
					if strings.Contains(strings.ToLower(originStr), "indeed") {
						filteredOrigins = append(filteredOrigins, o)
					}
				}
			}
		}
		genericState["origins"] = filteredOrigins
	}

	// 6. Serializar el estado limpio y guardarlo en el archivo
	filteredJSON, err := json.MarshalIndent(genericState, "", "    ")
	if err != nil {
		return fmt.Errorf("error serializando cookies filtradas: %w", err)
	}

	if err := os.WriteFile(cookiesPath, filteredJSON, 0644); err != nil {
		return fmt.Errorf("error guardando archivo de cookies: %w", err)
	}

	im.config.Log.Comentario("SUCCESS", fmt.Sprintf("💾 Cookies de Indeed guardadas en: %s", cookiesPath))
	return nil
}

// SearchJobs ejecuta la búsqueda
func (im *IndeedManager) SearchJobs(what, where string) error {
	im.config.Log.Comentario("INFO", fmt.Sprintf("🔍 Buscando: '%s' en '%s'", what, where))

	if err := im.page.Locator(im.indeedCfg.Search.InputWhat).Fill(what); err != nil {
		return err
	}
	if err := im.page.Locator(im.indeedCfg.Search.InputWhere).Fill(where); err != nil {
		return err
	}
	if err := im.page.Locator(im.indeedCfg.Search.BtnSearch).First().Click(); err != nil {
		return err
	}

	im.page.WaitForSelector("//div[contains(@class, 'jobsearch-ResultsList')]", playwright.PageWaitForSelectorOptions{
		State:   playwright.WaitForSelectorStateVisible,
		Timeout: playwright.Float(10000),
	})

	im.config.Log.Comentario("SUCCESS", "✅ Búsqueda completada")
	return nil
}

// IsLoggedIn verifica autenticación
func (im *IndeedManager) IsLoggedIn() bool {
	indicators := []string{
		"//a[contains(@href, '/profile')]",
		"//span[contains(text(), 'Mi perfil')]",
	}
	for _, xpath := range indicators {
		if count, _ := im.page.Locator(xpath).Count(); count > 0 {
			return true
		}
	}
	return false
}

// EnsureLoggedIn verifica el estado de la sesión en la UI y ejecuta el login SOLO si el botón "Ingresar" es visible
func (im *IndeedManager) EnsureLoggedIn(email, username string) error {
	im.config.Log.Comentario("INFO", fmt.Sprintf("👤 Verificando sesión para: %s", email))

	// 1️⃣ Navegar al portal
	if _, err := im.page.Goto(im.indeedCfg.URL, playwright.PageGotoOptions{
		WaitUntil: playwright.WaitUntilStateNetworkidle,
	}); err != nil {
		return fmt.Errorf("error navegando a Indeed: %w", err)
	}

	// 2️⃣ Verificar si existe el botón "Ingresar" en el navbar
	btnLoginXPath := im.indeedCfg.XPath["btn_ingresar_login"]
	btnLogin := im.page.Locator(btnLoginXPath)

	count, err := btnLogin.Count()
	if err != nil {
		return fmt.Errorf("error verificando botón de login: %w", err)
	}

	// 3️⃣ Si el botón NO existe (count == 0), asumimos que ya está logueado
	if count == 0 {
		im.config.Log.Comentario("SUCCESS", "✅ Sesión activa detectada (Botón 'Ingresar' no visible). Omitiendo login.")
		return nil
	}

	// 4️⃣ Si el botón SÍ existe, debemos iniciar sesión obligatoriamente
	im.config.Log.Comentario("INFO", "🔐 Sesión no detectada (Botón 'Ingresar' visible). Iniciando flujo de login...")
	return im.performLogin(email, username)
}

// performLogin ejecuta el flujo lineal de autenticación
func (im *IndeedManager) performLogin(email, username string) error {
	im.config.Log.Comentario("INFO", fmt.Sprintf("🔐 Iniciando login en Indeed para: %s", email))

	// 1️⃣ Hacer clic en "Ingresar"
	if err := im.page.Locator(im.indeedCfg.XPath["btn_ingresar_login"]).First().Click(); err != nil {
		return fmt.Errorf("error haciendo clic en 'Ingresar': %w", err)
	}

	// 2️⃣ Esperar y llenar el correo
	im.page.WaitForSelector(im.indeedCfg.XPath["input_correo"], playwright.PageWaitForSelectorOptions{
		State: playwright.WaitForSelectorStateVisible,
	})
	if err := im.page.Locator(im.indeedCfg.XPath["input_correo"]).Fill(email); err != nil {
		return fmt.Errorf("error llenando email: %w", err)
	}

	// 3️⃣ Hacer clic en "Continuar"
	if err := im.page.Locator(im.indeedCfg.XPath["btn_continuar"]).First().Click(); err != nil {
		return fmt.Errorf("error haciendo clic en 'Continuar': %w", err)
	}

	// 4️⃣ Esperar y presionar "Iniciar sesión con código"
	im.config.Log.Comentario("INFO", "⏳ Esperando botón de inicio de sesión con código...")
	im.page.WaitForSelector(im.indeedCfg.XPath["btn_iniciar_codigo"], playwright.PageWaitForSelectorOptions{
		State: playwright.WaitForSelectorStateVisible,
	})
	if err := im.page.Locator(im.indeedCfg.XPath["btn_iniciar_codigo"]).First().Click(); err != nil {
		return fmt.Errorf("error haciendo clic en 'Iniciar con código': %w", err)
	}

	// 5️⃣ Esperar input de código e ingresar
	im.config.Log.Comentario("INFO", "⏳ Esperando input de código...")
	im.page.WaitForSelector(im.indeedCfg.XPath["input_code"], playwright.PageWaitForSelectorOptions{
		State: playwright.WaitForSelectorStateVisible,
	})

	code := "123456" // ⚠️ Reemplazar con lógica real de obtención de código
	if err := im.page.Locator(im.indeedCfg.XPath["input_code"]).Fill(code); err != nil {
		return fmt.Errorf("error llenando código: %w", err)
	}

	// 6️⃣ Hacer clic en "Ingresar" para enviar el código
	im.config.Log.Comentario("INFO", "✅ Código ingresado, enviando formulario...")
	if err := im.page.Locator(im.indeedCfg.XPath["btn_ingresar"]).First().Click(); err != nil {
		return fmt.Errorf("error haciendo clic en 'Ingresar' (submit código): %w", err)
	}

	// 7️⃣ Esperar a que la sesión se establezca y guardar cookies
	im.page.WaitForTimeout(3000)
	im.config.Log.Comentario("SUCCESS", "✅ Login completado exitosamente")

	return im.SaveCookies(username)
}
