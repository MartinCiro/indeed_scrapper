package controller

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"
)

// OAuthService gestiona la autenticación OAuth2 con Gmail y la lectura de correos
type OAuthService struct {
	config *Config
}

// NewOAuthService crea una nueva instancia del servicio OAuth
func NewOAuthService(config *Config) *OAuthService {
	return &OAuthService{config: config}
}

// getClientSecretPath retorna la ruta del archivo client_secret.json del usuario
func (o *OAuthService) getClientSecretPath(username string) string {
	return filepath.Join("cookies", "users", username, "client_secret.json")
}

// getTokenPath retorna la ruta del archivo token.json del usuario
func (o *OAuthService) getTokenPath(username string) string {
	return filepath.Join("cookies", "users", username, "token.json")
}

// GetCodeFromGmail obtiene el código 2FA de Indeed desde el correo más reciente
func (o *OAuthService) GetCodeFromGmail(username string) (string, error) {
	o.config.Log.Comentario("INFO", fmt.Sprintf("📧 Conectando a Gmail para usuario: %s", username))

	// 1️⃣ Verificar que existe el client_secret.json
	clientSecretPath := o.getClientSecretPath(username)
	if _, err := os.Stat(clientSecretPath); os.IsNotExist(err) {
		return "", fmt.Errorf("❌ No se encontró client_secret.json en: %s. Este archivo es OBLIGATORIO", clientSecretPath)
	}

	// 2️⃣ Obtener el cliente HTTP autenticado
	client, err := o.getAuthenticatedClient(username)
	if err != nil {
		return "", fmt.Errorf("error obteniendo cliente autenticado: %w", err)
	}

	// 3️⃣ Conectar con Gmail API
	ctx := context.Background()
	srv, err := gmail.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return "", fmt.Errorf("error creando servicio Gmail: %w", err)
	}

	// 4️⃣ Buscar correos recientes de Indeed (últimos 10 minutos)
	o.config.Log.Comentario("INFO", "🔍 Buscando correo de Indeed en los últimos 10 minutos...")

	afterTime := time.Now().Add(-10 * time.Minute).Unix()
	query := fmt.Sprintf("from:noreply@indeed.com after:%d", afterTime)

	req := srv.Users.Messages.List("me").Q(query).MaxResults(1)
	resp, err := req.Do()
	if err != nil {
		return "", fmt.Errorf("error consultando Gmail: %w", err)
	}

	if len(resp.Messages) == 0 {
		return "", fmt.Errorf("no se encontró ningún correo de Indeed en los últimos 10 minutos")
	}

	// 5️⃣ Obtener el mensaje completo
	msg, err := srv.Users.Messages.Get("me", resp.Messages[0].Id).Format("full").Do()
	if err != nil {
		return "", fmt.Errorf("error obteniendo mensaje: %w", err)
	}

	// 6️⃣ Extraer el cuerpo del correo
	body := o.extractEmailBody(msg)
	if body == "" {
		return "", fmt.Errorf("no se pudo extraer el cuerpo del correo")
	}

	// 7️⃣ Buscar el código de 6 dígitos con regex
	code := o.extractCode(body)
	if code == "" {
		return "", fmt.Errorf("no se encontró un código de 6 dígitos en el correo")
	}

	o.config.Log.Comentario("SUCCESS", fmt.Sprintf("✅ Código 2FA extraído: %s", code))
	return code, nil
}

// getAuthenticatedClient obtiene un cliente HTTP con OAuth2 válido
func (o *OAuthService) getAuthenticatedClient(username string) (*http.Client, error) {
	clientSecretPath := o.getClientSecretPath(username)
	tokenPath := o.getTokenPath(username)

	// 1️⃣ Leer el client_secret.json
	b, err := os.ReadFile(clientSecretPath)
	if err != nil {
		return nil, fmt.Errorf("error leyendo client_secret.json: %w", err)
	}

	// 2️⃣ Configurar OAuth2
	config, err := google.ConfigFromJSON(b, gmail.GmailReadonlyScope)
	if err != nil {
		return nil, fmt.Errorf("error configurando OAuth: %w", err)
	}

	// 3️⃣ Intentar cargar el token existente
	tok, err := o.loadToken(tokenPath)
	if err != nil {
		// Si no existe o es inválido, solicitar nuevo token
		o.config.Log.Comentario("INFO", "🔐 No se encontró token válido. Iniciando flujo de autorización...")
		tok, err = o.getTokenFromWeb(config)
		if err != nil {
			return nil, fmt.Errorf("error obteniendo token desde web: %w", err)
		}
		// Guardar el nuevo token
		if err := o.saveToken(tokenPath, tok); err != nil {
			return nil, fmt.Errorf("error guardando token: %w", err)
		}
	}

	// 4️⃣ Crear cliente HTTP con el token
	return config.Client(context.Background(), tok), nil
}

// getTokenFromWeb solicita un nuevo token al usuario (flujo interactivo)
func (o *OAuthService) getTokenFromWeb(config *oauth2.Config) (*oauth2.Token, error) {
	authURL := config.AuthCodeURL("state-token", oauth2.AccessTypeOffline)

	fmt.Println("\n====================================================================")
	fmt.Println("🔐 AUTORIZACIÓN OAUTH2 REQUERIDA")
	fmt.Println("====================================================================")
	fmt.Printf("1️⃣  Abre esta URL en tu navegador:\n%v\n\n", authURL)
	fmt.Println("2️⃣  Autoriza el acceso a tu cuenta de Gmail")
	fmt.Println("3️⃣  Copia el código de autorización y pégalo aquí:")
	fmt.Print("👉 Código: ")

	var authCode string
	if _, err := fmt.Scan(&authCode); err != nil {
		return nil, fmt.Errorf("error leyendo código de autorización: %w", err)
	}

	tok, err := config.Exchange(context.Background(), authCode)
	if err != nil {
		return nil, fmt.Errorf("error intercambiando código por token: %w", err)
	}

	return tok, nil
}

// loadToken carga un token desde el archivo
func (o *OAuthService) loadToken(file string) (*oauth2.Token, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	tok := &oauth2.Token{}
	err = json.NewDecoder(f).Decode(tok)
	return tok, err
}

// saveToken guarda un token en el archivo
func (o *OAuthService) saveToken(file string, token *oauth2.Token) error {
	dir := filepath.Dir(file)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("error creando directorio: %w", err)
	}

	f, err := os.Create(file)
	if err != nil {
		return err
	}
	defer f.Close()

	return json.NewEncoder(f).Encode(token)
}

// extractEmailBody extrae el cuerpo del correo (prioriza text/plain)
func (o *OAuthService) extractEmailBody(msg *gmail.Message) string {
	var body string

	if msg.Payload != nil {
		// Buscar en las partes del mensaje
		body = o.findBodyInParts(msg.Payload.Parts)

		// Si no hay partes, usar el body directo
		if body == "" && msg.Payload.Body != nil && msg.Payload.Body.Data != "" {
			body = decodeBase64(msg.Payload.Body.Data)
		}
	}

	return body
}

// findBodyInParts busca recursivamente el cuerpo en las partes del mensaje
func (o *OAuthService) findBodyInParts(parts []*gmail.MessagePart) string {
	for _, part := range parts {
		if part.MimeType == "text/plain" && part.Body != nil && part.Body.Data != "" {
			return decodeBase64(part.Body.Data)
		}
		if len(part.Parts) > 0 {
			if body := o.findBodyInParts(part.Parts); body != "" {
				return body
			}
		}
	}
	return ""
}

// decodeBase64 decodifica el contenido base64 URL-safe de Gmail
func decodeBase64(data string) string {
	// Gmail usa base64 URL-safe sin padding
	data = strings.ReplaceAll(data, "-", "+")
	data = strings.ReplaceAll(data, "_", "/")

	// Agregar padding si es necesario
	switch len(data) % 4 {
	case 2:
		data += "=="
	case 3:
		data += "="
	}

	decoded, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return ""
	}
	return string(decoded)
}

// extractCode busca un código de 6 dígitos en el texto
func (o *OAuthService) extractCode(text string) string {
	// Regex para encontrar un número de 6 dígitos
	re := regexp.MustCompile(`\b\d{6}\b`)
	matches := re.FindAllString(text, -1)

	if len(matches) > 0 {
		// Retornar el primer código encontrado
		return matches[0]
	}
	return ""
}

// ListRecentEmails lista los últimos N correos (útil para debug)
func (o *OAuthService) ListRecentEmails(username string, maxResults int64) error {
	client, err := o.getAuthenticatedClient(username)
	if err != nil {
		return err
	}

	ctx := context.Background()
	srv, err := gmail.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return err
	}

	req := srv.Users.Messages.List("me").MaxResults(maxResults)
	resp, err := req.Do()
	if err != nil {
		return err
	}

	fmt.Printf("\n📬 Últimos %d correos:\n", len(resp.Messages))
	for i, msg := range resp.Messages {
		fullMsg, err := srv.Users.Messages.Get("me", msg.Id).Format("metadata").Do()
		if err != nil {
			continue
		}

		var from, subject string
		for _, header := range fullMsg.Payload.Headers {
			if header.Name == "From" {
				from = header.Value
			}
			if header.Name == "Subject" {
				subject = header.Value
			}
		}

		fmt.Printf("%d. De: %s | Asunto: %s\n", i+1, from, subject)
	}

	return nil
}
