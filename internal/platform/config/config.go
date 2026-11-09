// Package config carga la configuración de la aplicación desde variables de
// entorno y la valida al arrancar. Es el ÚNICO lugar que lee os.Getenv.
//
// Todas las variables están documentadas en .env.example (raíz del repo).
// Para añadir una variable: añadir el campo aquí, leerlo en Load, validarlo
// en validate y documentarlo en .env.example.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Config es la configuración completa de la app.
type Config struct {
	Env      string // "development" o "production"
	HTTPAddr string // dirección donde escucha la app, p. ej. ":8120"
	BaseURL  string // URL pública sin "/" final, p. ej. "https://radioclub25peru.com"
	DataDir  string // carpeta de datos (base SQLite y uploads)
	WebDir   string // si no está vacío, plantillas y estáticos se leen del disco (modo desarrollo)
	Location *time.Location

	CookieSecure  bool
	SessionTTL    time.Duration
	IPHashSalt    string
	InternalToken string // secreto compartido con MediaMTX para /internal/*
	MaxUploadMB   int

	// Icecast (radio)
	IcecastURL        string // URL interna, p. ej. http://icecast:8000
	IcecastMount      string // p. ej. /radio
	IcecastAdminUser  string
	IcecastAdminPass  string
	IcecastSourcePass string // se muestra al admin en "Cómo emitir"

	// MediaMTX (TV)
	MediaMTXAPIURL string // URL interna de la API, p. ej. http://mediamtx:9997
	MediaMTXPath   string // nombre del path de la TV, p. ej. "tv"

	// Datos públicos de ingesta, solo para mostrarlos en la ayuda del panel
	IngestHost      string // host al que se conecta la PC emisora
	IcecastPublicPt int    // puerto público de Icecast (8010)
	RTMPPublicPort  int    // 1935
	SRTPublicPort   int    // 8890

	// Ubicación de la emisora: la PC del estudio la usa para la temperatura locutada.
	StationLat float64
	StationLon float64

	// Superadmin inicial (opcional): si no existe ningún superadmin se crea con estos datos.
	BootstrapSuperEmail string
	BootstrapSuperPass  string
}

// IsProduction indica si se ejecuta en producción.
func (c Config) IsProduction() bool { return c.Env == "production" }

// DBPath es la ruta del archivo SQLite.
func (c Config) DBPath() string { return filepath.Join(c.DataDir, "radioclub25.db") }

// UploadsDir es la carpeta de archivos subidos.
func (c Config) UploadsDir() string { return filepath.Join(c.DataDir, "uploads") }

// Load lee la configuración del entorno.
func Load() (Config, error) {
	c := Config{
		Env:                 env("APP_ENV", "development"),
		HTTPAddr:            env("HTTP_ADDR", ":8120"),
		BaseURL:             strings.TrimRight(env("BASE_URL", "http://localhost:8120"), "/"),
		DataDir:             env("DATA_DIR", "./data"),
		WebDir:              env("WEB_DIR", ""),
		CookieSecure:        envBool("COOKIE_SECURE", false),
		SessionTTL:          time.Duration(envInt("SESSION_TTL_HOURS", 12)) * time.Hour,
		IPHashSalt:          env("IP_HASH_SALT", ""),
		InternalToken:       env("INTERNAL_TOKEN", ""),
		MaxUploadMB:         envInt("MAX_UPLOAD_MB", 100),
		IcecastURL:          strings.TrimRight(env("ICECAST_URL", "http://localhost:8010"), "/"),
		IcecastMount:        env("ICECAST_MOUNT", "/radio"),
		IcecastAdminUser:    env("ICECAST_ADMIN_USER", "admin"),
		IcecastAdminPass:    env("ICECAST_ADMIN_PASSWORD", ""),
		IcecastSourcePass:   env("ICECAST_SOURCE_PASSWORD", ""),
		MediaMTXAPIURL:      strings.TrimRight(env("MEDIAMTX_API_URL", "http://localhost:9997"), "/"),
		MediaMTXPath:        env("MEDIAMTX_TV_PATH", "tv"),
		IngestHost:          env("INGEST_HOST", "localhost"),
		IcecastPublicPt:     envInt("ICECAST_PUBLIC_PORT", 8010),
		RTMPPublicPort:      envInt("RTMP_PUBLIC_PORT", 1935),
		SRTPublicPort:       envInt("SRT_PUBLIC_PORT", 8890),
		StationLat:          envFloat("STATION_LAT", -11.1067), // Huacho
		StationLon:          envFloat("STATION_LON", -77.6050),
		BootstrapSuperEmail: env("SUPERADMIN_EMAIL", ""),
		BootstrapSuperPass:  env("SUPERADMIN_PASSWORD", ""),
	}
	loc, err := time.LoadLocation(env("APP_TIMEZONE", "America/Lima"))
	if err != nil {
		return c, fmt.Errorf("config: APP_TIMEZONE: %w", err)
	}
	c.Location = loc
	return c, c.validate()
}

func (c Config) validate() error {
	var errs []error
	if c.Env != "development" && c.Env != "production" {
		errs = append(errs, errors.New("APP_ENV debe ser development o production"))
	}
	if c.IsProduction() {
		if len(c.IPHashSalt) < 16 {
			errs = append(errs, errors.New("IP_HASH_SALT es obligatorio en producción (mínimo 16 caracteres)"))
		}
		if len(c.InternalToken) < 16 {
			errs = append(errs, errors.New("INTERNAL_TOKEN es obligatorio en producción (mínimo 16 caracteres)"))
		}
		if !c.CookieSecure {
			errs = append(errs, errors.New("COOKIE_SECURE debe ser true en producción"))
		}
	}
	if c.MaxUploadMB <= 0 {
		errs = append(errs, errors.New("MAX_UPLOAD_MB debe ser mayor que 0"))
	}
	if c.SessionTTL <= 0 {
		errs = append(errs, errors.New("SESSION_TTL_HOURS debe ser mayor que 0"))
	}
	if c.StationLat < -90 || c.StationLat > 90 || c.StationLon < -180 || c.StationLon > 180 {
		errs = append(errs, errors.New("STATION_LAT/STATION_LON fuera de rango"))
	}
	if !strings.HasPrefix(c.IcecastMount, "/") {
		errs = append(errs, errors.New("ICECAST_MOUNT debe empezar con /"))
	}
	if err := errors.Join(errs...); err != nil {
		return fmt.Errorf("config inválida:\n%w", err)
	}
	return nil
}

func env(key, def string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func envFloat(key string, def float64) float64 {
	if v := os.Getenv(key); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return def
}

func envBool(key string, def bool) bool {
	if v := os.Getenv(key); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return def
}
