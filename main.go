package main

import (
	"fmt"
	"go-indeed/controller"
	"os"
	"os/signal"
	"syscall"

	"github.com/mxschmitt/playwright-go"
)

func main() {
	fmt.Println("==================================================")
	fmt.Println("🌐 Gestor de Navegador Brave con Anti-Detección")
	fmt.Println("==================================================")

	// 1️⃣ Instanciar configuración
	config := controller.NewConfig()

	// 2️⃣ Instanciar BrowserManager (sin argumentos, la config va en Initialize)
	browserManager := controller.NewBrowserManager()

	config.Log.InicioProceso("BrowserManager")
	config.Log.Comentario("SUCCESS", "Servicios inicializados")

	// 3️⃣ Inicializar navegador (retorna la página lista para usar)
	page, err := browserManager.Initialize(config)
	if err != nil {
		config.Log.Error(fmt.Sprintf("Error inicializando navegador: %v", err), "Browser")
		fmt.Printf("❌ Error: %v\n", err)
		os.Exit(1)
	}

	// 4️⃣ Usar la página directamente con la API de Playwright
	fmt.Println("🔄 Navegando a YouTube...")
	_, err = page.Goto("https://www.youtube.com", playwright.PageGotoOptions{
		WaitUntil: playwright.WaitUntilStateDomcontentloaded,
	})
	if err != nil {
		config.Log.Error(fmt.Sprintf("Error navegando: %v", err), "Browser")
	}

	// Obtener título
	title, err := page.Title()
	if err == nil {
		fmt.Printf("✅ Título de la página: %s\n", title)
	}

	// Tomar screenshot
	fmt.Println("📸 Tomando screenshot...")
	_, err = page.Screenshot(playwright.PageScreenshotOptions{
		Path:     playwright.String("youtube_screenshot.png"),
		FullPage: playwright.Bool(true),
	})
	if err != nil {
		config.Log.Error(fmt.Sprintf("Error en screenshot: %v", err), "Browser")
	} else {
		fmt.Println("✅ Screenshot guardado en youtube_screenshot.png")
	}

	// Ejecutar JavaScript
	fmt.Println("🌐 Evaluando User-Agent...")
	result, err := page.Evaluate("() => navigator.userAgent")
	if err == nil {
		fmt.Printf("✅ User-Agent: %v\n", result)
	}

	// 5️⃣ Shutdown graceful
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	fmt.Println("\n📱 Navegador activo... (Presiona Ctrl+C para cerrar)")
	<-sigChan

	// 6️⃣ Cleanup
	config.Log.Comentario("INFO", "Recibida señal de terminación")
	browserManager.Close() // Close no retorna error en nuestra implementación
	config.Log.FinProceso("BrowserManager")
	fmt.Println("\n✅ Programa finalizado")
}
