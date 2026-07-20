package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"go-indeed/controller"
)

func main() {
	fmt.Println("==================================================")
	fmt.Println("🔍 Indeed Colombia - Automatización Multi-Usuario")
	fmt.Println("==================================================")

	config := controller.NewConfig()
	browserManager := controller.NewBrowserManager()

	config.Log.InicioProceso("IndeedManager")
	config.Log.Comentario("SUCCESS", "Servicios inicializados")

	page, err := browserManager.Initialize(config)
	if err != nil {
		config.Log.Error(fmt.Sprintf("Error inicializando navegador: %v", err), "Browser")
		os.Exit(1)
	}

	indeedManager := controller.NewIndeedManager(page, config)
	users := config.GetIndeedUsers()

	if len(users) == 0 {
		config.Log.Comentario("WARNING", "⚠️ No hay usuarios configurados en config.json")
	}

	// 🔄 LOOP PRINCIPAL: Iterar sobre cada usuario
	for _, email := range users {
		username := controller.ExtractUsername(email)

		config.Log.Comentario("INFO", fmt.Sprintf("👤 Procesando usuario: %s (Carpeta: %s)", email, username))

		// ✅ La decisión de omitir o hacer login la toma EnsureLoggedIn basándose en la UI
		if err := indeedManager.EnsureLoggedIn(email, username); err != nil {
			config.Log.Error(fmt.Sprintf("Error asegurando sesión para %s: %v", email, err), "Indeed")
			continue // Pasar al siguiente usuario si uno falla
		}

		// --- AQUÍ PUEDES AGREGAR LA LÓGICA DE BÚSQUEDA PARA ESTE USUARIO ---
		// if err := indeedManager.SearchJobs("Desarrollador Go", "Bogotá"); err == nil {
		//     jobs, _ := indeedManager.GetJobListings()
		//     fmt.Printf("Empleos para %s: %d\n", username, len(jobs))
		// }
	}

	// Shutdown graceful
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	fmt.Println("\n📱 Navegador activo... (Presiona Ctrl+C para cerrar)")
	<-sigChan

	config.Log.Comentario("INFO", "Recibida señal de terminación")
	browserManager.Close()
	config.Log.FinProceso("IndeedManager")
	fmt.Println("\n✅ Programa finalizado")
}
