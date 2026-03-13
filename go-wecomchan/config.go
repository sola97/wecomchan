package main

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"time"
)

var (
	WebPassword        = strings.TrimSpace(os.Getenv("WEB_PASSWORD"))
	FrontendDistDir    = GetEnvDefault("FRONTEND_DIST_DIR", "./frontend/dist")
	ListenAddr         = GetEnvDefault("LISTEN_ADDR", ":8080")
	BotConfigPath      = GetEnvDefault("BOT_CONFIG_PATH", "./data/bot-configs.json")
	MessageLogPath     = GetEnvDefault("MESSAGE_LOG_PATH", "./data/message-logs.jsonl")
	DefaultRouteSuffix = normalizeRouteSuffix(GetEnvDefault("DEFAULT_ROUTE_SUFFIX", "wecomchan"))

	httpClient = &http.Client{Timeout: 20 * time.Second}
	appState   *AppState
)

type AppState struct {
	BotConfigs  *BotConfigStore
	MessageLogs *MessageLogStore
}

const (
	ContentTypeBinary   = "application/octet-stream"
	ContentTypeForm     = "application/x-www-form-urlencoded"
	ContentTypeFormData = "multipart/form-data"
	ContentTypeJSON     = "application/json"
)

type apiError struct {
	Error string `json:"error"`
}

// GetEnvDefault returns the configured env value or a default.
func GetEnvDefault(key, defVal string) string {
	val, ok := os.LookupEnv(key)
	if !ok {
		return defVal
	}
	return val
}

func LookupEnvTrim(key string) (string, bool) {
	value, ok := os.LookupEnv(key)
	if !ok {
		return "", false
	}
	return strings.TrimSpace(value), true
}

func writeJSON(res http.ResponseWriter, status int, payload interface{}) {
	res.Header().Set("Content-Type", "application/json; charset=utf-8")
	res.WriteHeader(status)
	if err := json.NewEncoder(res).Encode(payload); err != nil {
		http.Error(res, `{"error":"failed to encode response"}`, http.StatusInternalServerError)
	}
}

func writeJSONError(res http.ResponseWriter, status int, message string) {
	writeJSON(res, status, apiError{Error: message})
}
