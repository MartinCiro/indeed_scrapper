// controller/Log.go
package controller

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	logAncho         = 120
	logFormatoTiempo = "2006-01-02 15:04:05"
)

// Log sistema de logging con escritura thread-safe
type Log struct {
	rutaBase        string
	rutaProcesos    string
	rutaErrores     string
	archivoProcesos string
	archivoErrores  string
	mu              sync.Mutex
}

// NewLog crea una nueva instancia de Log con las rutas configuradas
func NewLog() *Log {
	fechaActual := time.Now().Format("2006-01-02")

	cwd, err := os.Getwd()
	if err != nil {
		cwd = "."
	}

	rutaBase := cwd
	rutaProcesos := filepath.Join(rutaBase, "logs", "procesos")
	rutaErrores := filepath.Join(rutaBase, "logs", "errores")

	// Crear directorios si no existen
	os.MkdirAll(rutaProcesos, 0755)
	os.MkdirAll(rutaErrores, 0755)

	archivoProcesos := filepath.Join(rutaProcesos, fmt.Sprintf("LogProcesos_%s.txt", fechaActual))
	archivoErrores := filepath.Join(rutaErrores, fmt.Sprintf("LogErrores_%s.txt", fechaActual))

	return &Log{
		rutaBase:        rutaBase,
		rutaProcesos:    rutaProcesos,
		rutaErrores:     rutaErrores,
		archivoProcesos: archivoProcesos,
		archivoErrores:  archivoErrores,
	}
}

func (l *Log) tiempoActual() string {
	return time.Now().Format(logFormatoTiempo)
}

func (l *Log) formatearMensaje(lineas ...string) string {
	var sb strings.Builder
	sb.WriteString(strings.Repeat("=", logAncho) + "\n")
	for _, linea := range lineas {
		// ← FIX: Truncar línea si es muy larga para evitar Repeat count negativo
		maxLen := logAncho - 4
		if len(linea) > maxLen {
			linea = linea[:maxLen-3] + "..."
		}

		// Padding: "| contenido |"
		padded := linea + strings.Repeat(" ", maxLen-len(linea))
		sb.WriteString(fmt.Sprintf("| %s |\n", padded))
	}
	sb.WriteString(strings.Repeat("=", logAncho) + "\n")
	return sb.String()
}

func (l *Log) escribirLog(archivo string, mensaje string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	f, err := os.OpenFile(archivo, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error abriendo log %s: %v\n", archivo, err)
		return
	}
	defer f.Close()

	f.WriteString(mensaje + "\n")
}

// InicioProceso registra el inicio de un proceso
func (l *Log) InicioProceso(nombreAplicacion ...string) {
	nombre := "Proceso"
	if len(nombreAplicacion) > 0 && nombreAplicacion[0] != "" {
		nombre = nombreAplicacion[0]
	}
	mensaje := l.formatearMensaje(
		fmt.Sprintf("INICIO DE EJECUCIÓN - %s - %s", nombre, l.tiempoActual()),
	)
	l.escribirLog(l.archivoProcesos, mensaje)
}

// FinProceso registra la finalización de un proceso
func (l *Log) FinProceso(nombreAplicacion ...string) {
	nombre := "Proceso"
	if len(nombreAplicacion) > 0 && nombreAplicacion[0] != "" {
		nombre = nombreAplicacion[0]
	}
	mensaje := l.formatearMensaje(
		fmt.Sprintf("FIN DE EJECUCIÓN - %s - %s", nombre, l.tiempoActual()),
	)
	l.escribirLog(l.archivoProcesos, mensaje)
}

// Proceso registra un proceso específico
func (l *Log) Proceso(nombreProceso string) {
	mensaje := fmt.Sprintf("| Ejecutando: %-80s | Hora: %s |", nombreProceso, l.tiempoActual())
	l.escribirLog(l.archivoProcesos, mensaje)
}

// Comentario registra un comentario con nivel de severidad
func (l *Log) Comentario(nivel string, mensaje string) {
	contenido := l.formatearMensaje(
		fmt.Sprintf("%s: %s", strings.ToUpper(nivel), mensaje),
		fmt.Sprintf("Hora: %s", l.tiempoActual()),
	)
	l.escribirLog(l.archivoProcesos, contenido)
}

// Error registra un error ocurrido
func (l *Log) Error(descripcionError string, proceso ...string) {
	nombreProceso := "Proceso no especificado"
	if len(proceso) > 0 && proceso[0] != "" {
		nombreProceso = fmt.Sprintf("Proceso: %s", proceso[0])
	}

	contenido := l.formatearMensaje(
		fmt.Sprintf("ERROR DETECTADO - %s", l.tiempoActual()),
		nombreProceso,
		fmt.Sprintf("Detalle: %s", descripcionError),
	)
	l.escribirLog(l.archivoErrores, contenido)
	l.escribirLog(l.archivoProcesos, contenido) // También en log de procesos
}

// Separador añade un separador visual al log
func (l *Log) Separador() {
	l.escribirLog(l.archivoProcesos, strings.Repeat("=", logAncho))
}

func (l *Log) cleanupOldLogs() {
	// Borrar logs de más de 7 días
	cutoff := time.Now().AddDate(0, 0, -7)

	filepath.Walk(l.rutaProcesos, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() && info.ModTime().Before(cutoff) {
			os.Remove(path)
		}
		return nil
	})
}
