package controller

import (
	"fmt"
	"os"
	"runtime"
	"strings"
	"sync"

	"github.com/mxschmitt/playwright-go"
)

// BrowserManager gestiona el ciclo de vida del navegador Brave con anti-detección.
// Thread-safe: usa mutex para proteger estado compartido.
type BrowserManager struct {
	mu          sync.Mutex
	pw          *playwright.Playwright
	context     playwright.BrowserContext // ← SIN asterisco (*), es una interfaz
	page        playwright.Page
	initialized bool
	config      *Config
}

// NewBrowserManager crea una nueva instancia del gestor
func NewBrowserManager() *BrowserManager {
	return &BrowserManager{}
}

// Initialize lanza el navegador Brave con anti-detección y perfil persistente.
func (bm *BrowserManager) Initialize(config *Config) (playwright.Page, error) {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	bm.config = config

	// Si ya está vivo, reutilizar
	if bm.isAliveLocked() {
		if bm.page == nil || bm.page.IsClosed() {
			var err error
			bm.page, err = bm.context.NewPage()
			if err != nil {
				return nil, fmt.Errorf("crear nueva página en contexto existente: %w", err)
			}
			config.Log.Comentario("INFO", "📑 Nueva página creada en contexto existente")
		}

		_ = bm.page.BringToFront()
		width, height := getScreenSize()
		_ = bm.page.SetViewportSize(width, height)
		return bm.page, nil
	}

	config.Log.Comentario("INFO", "🔄 Inicializando navegador Brave...")
	bm.cleanupLocked()

	// 1️⃣ Iniciar Playwright
	pw, err := playwright.Run()
	if err != nil {
		config.Log.Error(fmt.Sprintf("No se pudo iniciar Playwright: %v", err), "Inicializando el navegador")
		return nil, err
	}
	bm.pw = pw

	// 2️⃣ Preparar directorio de usuario (perfil persistente)
	userDataDir := expandUser(config.GetUserBrowserDirectory())
	if err := os.MkdirAll(userDataDir, 0755); err != nil {
		config.Log.Error(fmt.Sprintf("No se pudo crear directorio de usuario: %v", err), "Inicializando el navegador")
		bm.cleanupLocked()
		return nil, err
	}

	// 3️⃣ Ruta del ejecutable
	braveExec := config.GetChromePath()
	if braveExec == "" {
		config.Log.Error("❌ No se encontró la ruta de Brave/Chrome", "Inicializando el navegador")
		bm.cleanupLocked()
		return nil, fmt.Errorf("no se encontró ejecutable de Chrome/Brave")
	}

	// 4️⃣ Opciones de lanzamiento con anti-detección
	platform := "Linux"
	if runtime.GOOS == "windows" {
		platform = "Windows"
	} else if runtime.GOOS == "darwin" {
		platform = "macOS"
	}

	launchOpts := playwright.BrowserTypeLaunchPersistentContextOptions{
		Headless:       playwright.Bool(strings.ToLower(config.Headless) == "true"),
		ExecutablePath: playwright.String(braveExec),
		Args: []string{
			"--disable-blink-features=AutomationControlled",
			"--disable-features=IsolateOrigins,site-per-process,TranslateUI,PrivacySandboxFirstPartySets",
			"--disable-dev-shm-usage",
			"--force-color-profile=srgb",
			"--disable-accelerated-2d-canvas",
			"--no-first-run",
			"--no-default-browser-check",
			"--disable-infobars",
			"--disable-background-networking",
			"--disable-sync",
			"--lang=es-ES",
			"--accept-lang=es-ES,es;q=0.9,en-US;q=0.8,en;q=0.7",
			"--start-maximized",
		},
		IgnoreDefaultArgs: []string{
			"--enable-automation",
		},
		NoViewport: playwright.Bool(true),
	}

	// 5️⃣ Crear contexto persistente
	ctx, err := pw.Chromium.LaunchPersistentContext(userDataDir, launchOpts)
	if err != nil {
		config.Log.Error(fmt.Sprintf("No se pudo lanzar contexto persistente: %v", err), "Inicializando el navegador")
		bm.cleanupLocked()
		return nil, err
	}
	bm.context = ctx

	// 6️⃣ Script stealth (anti-detección)
	stealthScript := `
		delete Object.getPrototypeOf(navigator).webdriver;
		Object.defineProperty(navigator, 'webdriver', { get: () => undefined, configurable: true });
		if (!window.chrome) {
			window.chrome = { runtime: { id: 'fake-id', connect: () => {}, sendMessage: () => {} } };
		}
		if (!navigator.plugins || navigator.plugins.length === 0) {
			const plugins = {
				0: { name: 'Chrome PDF Plugin', filename: 'internal-pdf-viewer' },
				1: { name: 'Chrome PDF Viewer', filename: 'mhjfbmdgcfjbbpaeojofohoefgiehjai' },
				2: { name: 'Native Client', filename: 'internal-nacl-plugin' },
				length: 3
			};
			Object.setPrototypeOf(plugins, PluginArray.prototype);
			Object.defineProperty(navigator, 'plugins', { get: () => plugins });
		}
		Object.defineProperty(navigator, 'languages', { get: () => ['es-ES', 'es', 'en'] });
		Object.defineProperty(navigator, 'hardwareConcurrency', { get: () => 8 });
		Object.defineProperty(navigator, 'deviceMemory', { get: () => 8 });
		delete window.__playwright;
		delete window.__pw_manual;
	`

	// ✅ FIX: Usar playwright.Script{Content: playwright.String(...)}
	if err := ctx.AddInitScript(playwright.Script{
		Content: playwright.String(stealthScript),
	}); err != nil {
		config.Log.Comentario("WARNING", fmt.Sprintf("⚠️ No se pudo inyectar script stealth: %v", err))
	}

	// 7️⃣ Headers HTTP realistas
	_ = ctx.SetExtraHTTPHeaders(map[string]string{
		"Accept-Language":    "es-ES,es;q=0.9,en;q=0.8",
		"Sec-Ch-Ua":          `"Brave";v="122", "Not:A-Brand";v="24", "Chromium";v="122"`,
		"Sec-Ch-Ua-Mobile":   "?0",
		"Sec-Ch-Ua-Platform": fmt.Sprintf(`"%s"`, platform),
	})

	// 8️⃣ Obtener o crear página principal
	bm.page, err = bm.ensureFirstTabLocked()
	if err != nil {
		config.Log.Error(fmt.Sprintf("No se pudo asegurar pestaña principal: %v", err), "Inicializando el navegador")
		bm.cleanupLocked()
		return nil, err
	}

	bm.initialized = true
	config.Log.Comentario("SUCCESS", "🌐 Brave Browser iniciado con anti-detección")
	return bm.page, nil
}

func (bm *BrowserManager) ensureFirstTabLocked() (playwright.Page, error) {
	pages := bm.context.Pages()

	if len(pages) == 0 {
		p, err := bm.context.NewPage()
		if err != nil {
			return nil, err
		}
		bm.config.Log.Comentario("INFO", "📑 Creada nueva pestaña principal")
		return p, nil
	}

	page := pages[0]

	if len(pages) > 1 {
		bm.config.Log.Comentario("INFO", fmt.Sprintf("🧹 Cerrando %d pestañas adicionales", len(pages)-1))
		for i := len(pages) - 1; i > 0; i-- {
			_ = pages[i].Close()
		}
	}

	_ = page.BringToFront()
	return page, nil
}

func (bm *BrowserManager) GetPage() playwright.Page {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	if !bm.initialized || bm.page == nil || bm.page.IsClosed() {
		return nil
	}
	return bm.page
}

func (bm *BrowserManager) Close() {
	bm.mu.Lock()
	defer bm.mu.Unlock()
	bm.cleanupLocked()
}

func (bm *BrowserManager) IsInitialized() bool {
	bm.mu.Lock()
	defer bm.mu.Unlock()
	return bm.initialized && bm.page != nil && !bm.page.IsClosed()
}

func (bm *BrowserManager) isAliveLocked() bool {
	if bm.pw == nil || bm.context == nil {
		return false
	}
	browser := bm.context.Browser()
	// ✅ FIX: Browser usa IsConnected(), no IsClosed()
	if browser != nil && !browser.IsConnected() {
		return false
	}
	if bm.page != nil && bm.page.IsClosed() {
		return false
	}
	return true
}

func (bm *BrowserManager) cleanupLocked() {
	log := bm.config

	if bm.page != nil && !bm.page.IsClosed() {
		if err := bm.page.Close(); err != nil && log != nil {
			log.Log.Comentario("WARNING", fmt.Sprintf("⚠️ Error cerrando página: %v", err))
		}
	}

	// ✅ FIX: BrowserContext SÍ tiene IsClosed()
	if bm.context != nil && !bm.context.IsClosed() {
		if err := bm.context.Close(); err != nil && log != nil {
			log.Log.Comentario("WARNING", fmt.Sprintf("⚠️ Error cerrando contexto: %v", err))
		}
	}

	if bm.pw != nil {
		if err := bm.pw.Stop(); err != nil && log != nil {
			log.Log.Comentario("WARNING", fmt.Sprintf("⚠️ Error deteniendo Playwright: %v", err))
		}
	}

	if log != nil {
		log.Log.Comentario("INFO", "🛑 Navegador cerrado")
	}

	bm.pw = nil
	bm.context = nil
	bm.page = nil
	bm.initialized = false
}

func getScreenSize() (int, int) {
	return 1920, 1080
}
