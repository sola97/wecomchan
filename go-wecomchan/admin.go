package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	adminSessionCookieName = "wecomchan_admin_session"
	adminSessionTTL        = 12 * time.Hour
)

type adminLoginRequest struct {
	Password string `json:"password"`
}

func registerAdminRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/admin/login", adminLoginHandler)
	mux.HandleFunc("/api/admin/logout", adminLogoutHandler)
	mux.HandleFunc("/api/admin/session", adminSessionHandler)
	mux.Handle("/api/admin/docs", requireAdmin(http.HandlerFunc(adminDocsHandler)))
	mux.Handle("/api/admin/logs/messages", requireAdmin(http.HandlerFunc(adminMessageLogsHandler)))
	mux.Handle("/api/admin/settings/redis", requireAdmin(http.HandlerFunc(adminRedisSettingsHandler)))
	mux.Handle("/api/admin/configs", requireAdmin(http.HandlerFunc(adminConfigsHandler)))
	mux.Handle("/api/admin/configs/", requireAdmin(http.HandlerFunc(adminConfigItemHandler)))
	mux.Handle("/api/admin/execute/get-text", requireAdmin(http.HandlerFunc(adminExecuteGetTextHandler)))
	mux.Handle("/api/admin/execute/post-message", requireAdmin(http.HandlerFunc(adminExecutePostMessageHandler)))
	mux.Handle("/api/admin/execute/image", requireAdmin(http.HandlerFunc(adminExecuteImageHandler)))
	mux.Handle("/api/admin/execute/base62", requireAdmin(http.HandlerFunc(adminExecuteTextImageHandler)))

	adminUI := newAdminUIHandler(FrontendDistDir)
	mux.Handle("/admin", adminUI)
	mux.Handle("/admin/", adminUI)
}

func requireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		authenticated, err := isAdminAuthenticated(req)
		if err != nil {
			writeJSONError(res, http.StatusUnauthorized, err.Error())
			return
		}
		if !authenticated {
			writeJSONError(res, http.StatusUnauthorized, "admin login required")
			return
		}
		next.ServeHTTP(res, req)
	})
}

func isAdminAuthenticated(req *http.Request) (bool, error) {
	if strings.TrimSpace(WebPassword) == "" {
		return false, fmt.Errorf("WEB_PASSWORD is not configured")
	}

	cookie, err := req.Cookie(adminSessionCookieName)
	if err != nil {
		if err == http.ErrNoCookie {
			return false, nil
		}
		return false, err
	}

	payloadBytes, err := base64.RawURLEncoding.DecodeString(cookie.Value)
	if err != nil {
		return false, nil
	}

	parts := strings.SplitN(string(payloadBytes), ".", 2)
	if len(parts) != 2 {
		return false, nil
	}

	expiresAt, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return false, nil
	}
	if time.Now().Unix() > expiresAt {
		return false, nil
	}

	expectedSignature := signAdminSession(parts[0])
	if subtle.ConstantTimeCompare([]byte(expectedSignature), []byte(parts[1])) != 1 {
		return false, nil
	}
	return true, nil
}

func signAdminSession(payload string) string {
	mac := hmac.New(sha256.New, []byte(WebPassword))
	_, _ = io.WriteString(mac, payload)
	return hex.EncodeToString(mac.Sum(nil))
}

func issueAdminSessionCookie() *http.Cookie {
	expiresAt := time.Now().Add(adminSessionTTL)
	expiresValue := strconv.FormatInt(expiresAt.Unix(), 10)
	signature := signAdminSession(expiresValue)
	value := base64.RawURLEncoding.EncodeToString([]byte(expiresValue + "." + signature))
	return &http.Cookie{
		Name:     adminSessionCookieName,
		Value:    value,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Expires:  expiresAt,
		MaxAge:   int(adminSessionTTL.Seconds()),
	}
}

func clearAdminSessionCookie() *http.Cookie {
	return &http.Cookie{
		Name:     adminSessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
	}
}

func adminLoginHandler(res http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		writeJSONError(res, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if strings.TrimSpace(WebPassword) == "" {
		writeJSONError(res, http.StatusServiceUnavailable, "WEB_PASSWORD is not configured")
		return
	}

	var payload adminLoginRequest
	switch {
	case hasContentType(req, ContentTypeJSON):
		if err := decodeAdminLogin(req, &payload); err != nil {
			writeJSONError(res, http.StatusBadRequest, err.Error())
			return
		}
	default:
		if err := req.ParseForm(); err != nil {
			writeJSONError(res, http.StatusBadRequest, "failed to parse request")
			return
		}
		payload.Password = req.FormValue("password")
	}

	if subtle.ConstantTimeCompare([]byte(payload.Password), []byte(WebPassword)) != 1 {
		writeJSONError(res, http.StatusUnauthorized, "password is invalid")
		return
	}

	http.SetCookie(res, issueAdminSessionCookie())
	writeJSON(res, http.StatusOK, map[string]interface{}{
		"authenticated": true,
		"configured":    true,
	})
}

func adminLogoutHandler(res http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		writeJSONError(res, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	http.SetCookie(res, clearAdminSessionCookie())
	writeJSON(res, http.StatusOK, map[string]interface{}{
		"authenticated": false,
	})
}

func adminSessionHandler(res http.ResponseWriter, req *http.Request) {
	configured := strings.TrimSpace(WebPassword) != ""
	authenticated, err := isAdminAuthenticated(req)
	if err != nil && configured {
		writeJSONError(res, http.StatusInternalServerError, err.Error())
		return
	}

	configCount := 0
	redisConfig := RedisConfig{}
	if appState != nil && appState.BotConfigs != nil {
		configCount = appState.BotConfigs.Count()
		redisConfig = appState.BotConfigs.RedisConfig()
	}

	writeJSON(res, http.StatusOK, map[string]interface{}{
		"configured":        configured,
		"authenticated":     authenticated,
		"dist_available":    adminDistAvailable(),
		"config_count":      configCount,
		"default_route":     DefaultRouteSuffix,
		"config_store_path": BotConfigPath,
		"redis":             redisConfig,
	})
}

func adminConfigsHandler(res http.ResponseWriter, req *http.Request) {
	switch req.Method {
	case http.MethodGet:
		redisConfig := RedisConfig{}
		if appState != nil && appState.BotConfigs != nil {
			redisConfig = appState.BotConfigs.RedisConfig()
		}
		writeJSON(res, http.StatusOK, map[string]interface{}{
			"configs":              appState.BotConfigs.List(),
			"default_route_suffix": DefaultRouteSuffix,
			"redis":                redisConfig,
		})
	case http.MethodPost:
		var cfg BotConfig
		if err := jsonNewDecoder(req.Body).Decode(&cfg); err != nil {
			writeJSONError(res, http.StatusBadRequest, "failed to parse config payload")
			return
		}
		saved, err := appState.BotConfigs.Upsert("", cfg)
		if err != nil {
			writeJSONError(res, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(res, http.StatusOK, map[string]interface{}{
			"config": saved,
		})
	default:
		writeJSONError(res, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func adminRedisSettingsHandler(res http.ResponseWriter, req *http.Request) {
	switch req.Method {
	case http.MethodGet:
		redisConfig := RedisConfig{}
		if appState != nil && appState.BotConfigs != nil {
			redisConfig = appState.BotConfigs.RedisConfig()
		}
		writeJSON(res, http.StatusOK, map[string]interface{}{
			"redis": redisConfig,
		})
	case http.MethodPut:
		var cfg RedisConfig
		if err := jsonNewDecoder(req.Body).Decode(&cfg); err != nil {
			writeJSONError(res, http.StatusBadRequest, "failed to parse redis payload")
			return
		}
		saved, err := appState.BotConfigs.UpdateRedisConfig(cfg)
		if err != nil {
			writeJSONError(res, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(res, http.StatusOK, map[string]interface{}{
			"redis": saved,
		})
	default:
		writeJSONError(res, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func adminMessageLogsHandler(res http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		writeJSONError(res, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	days := 7
	if raw := strings.TrimSpace(req.URL.Query().Get("days")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed <= 0 {
			writeJSONError(res, http.StatusBadRequest, "days must be a positive integer")
			return
		}
		days = parsed
	}

	limit := defaultMessageLogLimit
	if raw := strings.TrimSpace(req.URL.Query().Get("limit")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed <= 0 {
			writeJSONError(res, http.StatusBadRequest, "limit must be a positive integer")
			return
		}
		limit = parsed
	}

	routeSuffix := normalizeRouteSuffix(req.URL.Query().Get("route_suffix"))
	entries, err := appState.MessageLogs.ListRecent(time.Duration(days)*24*time.Hour, limit*4)
	if err != nil {
		writeJSONError(res, http.StatusInternalServerError, err.Error())
		return
	}

	filtered := make([]MessageLogEntry, 0, limit)
	for _, entry := range entries {
		if routeSuffix != "" && entry.RouteSuffix != routeSuffix {
			continue
		}
		filtered = append(filtered, entry)
		if len(filtered) >= limit {
			break
		}
	}

	writeJSON(res, http.StatusOK, map[string]interface{}{
		"days":        days,
		"limit":       limit,
		"route_suffix": routeSuffix,
		"items":       filtered,
	})
}

func adminConfigItemHandler(res http.ResponseWriter, req *http.Request) {
	routeSuffix, err := configRouteSuffixFromPath(req.URL.Path)
	if err != nil {
		writeJSONError(res, http.StatusBadRequest, err.Error())
		return
	}

	switch req.Method {
	case http.MethodGet:
		cfg, ok := appState.BotConfigs.Get(routeSuffix)
		if !ok {
			writeJSONError(res, http.StatusNotFound, "config not found")
			return
		}
		writeJSON(res, http.StatusOK, map[string]interface{}{"config": cfg})
	case http.MethodPut:
		var cfg BotConfig
		if err := jsonNewDecoder(req.Body).Decode(&cfg); err != nil {
			writeJSONError(res, http.StatusBadRequest, "failed to parse config payload")
			return
		}
		saved, err := appState.BotConfigs.Upsert(routeSuffix, cfg)
		if err != nil {
			writeJSONError(res, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(res, http.StatusOK, map[string]interface{}{"config": saved})
	case http.MethodDelete:
		if err := appState.BotConfigs.Delete(routeSuffix); err != nil {
			writeJSONError(res, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(res, http.StatusOK, map[string]interface{}{"deleted": routeSuffix})
	default:
		writeJSONError(res, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func configRouteSuffixFromPath(path string) (string, error) {
	value := strings.TrimPrefix(path, "/api/admin/configs/")
	value, err := url.PathUnescape(value)
	if err != nil {
		return "", fmt.Errorf("invalid config path")
	}
	value = normalizeRouteSuffix(value)
	if value == "" || strings.Contains(value, "/") {
		return "", fmt.Errorf("route_suffix is required")
	}
	return value, nil
}

func resolveAdminTargetConfig(routeSuffix string) (BotConfig, error) {
	cfg, err := resolveBotConfig(routeSuffix)
	if err != nil {
		return BotConfig{}, err
	}
	return cfg, nil
}

func adminDocsHandler(res http.ResponseWriter, req *http.Request) {
	origin := requestOrigin(req)
	routeSuffix := normalizeRouteSuffix(req.URL.Query().Get("route_suffix"))

	pathRoute := "<route_suffix>"
	callbackRoute := "<route_suffix>/callback"
	if cfg, err := resolveAdminTargetConfig(routeSuffix); err == nil {
		pathRoute = cfg.RouteSuffix
		callbackRoute = cfg.RouteSuffix + "/callback"
	}

	writeJSON(res, http.StatusOK, map[string]interface{}{
		"routes": []map[string]interface{}{
			{
				"title":       "GET 发送文本",
				"method":      "GET",
				"path":        origin + "/" + pathRoute,
				"description": "通过查询参数发送文本消息。",
				"params": []map[string]string{
					{"name": "sendkey", "location": "query", "required": "true", "description": "发送密钥"},
					{"name": "msg", "location": "query", "required": "true", "description": "文本内容"},
					{"name": "msg_type", "location": "query", "required": "true", "description": "固定为 text"},
				},
			},
			{
				"title":       "POST 发送文本/Markdown",
				"method":      "POST",
				"path":        origin + "/" + pathRoute,
				"description": "支持 application/json、x-www-form-urlencoded、multipart/form-data。",
				"params": []map[string]string{
					{"name": "sendkey", "location": "body", "required": "true", "description": "发送密钥"},
					{"name": "msg", "location": "body", "required": "true", "description": "消息内容"},
					{"name": "msg_type", "location": "body", "required": "true", "description": "text 或 markdown"},
				},
			},
			{
				"title":       "POST 图片发送",
				"method":      "POST",
				"path":        origin + "/" + pathRoute,
				"description": "使用 multipart/form-data 上传图片字段 media。",
				"params": []map[string]string{
					{"name": "sendkey", "location": "body/query", "required": "true", "description": "发送密钥"},
					{"name": "msg_type", "location": "body/query", "required": "true", "description": "固定为 image"},
					{"name": "media", "location": "multipart", "required": "true", "description": "图片文件，最大 2MB"},
				},
			},
			{
				"title":       "POST 图文双发",
				"method":      "POST",
				"path":        origin + "/" + pathRoute,
				"description": "沿用文本接口，增加 image 字段后自动拆成 text 与 image 两条消息发送。",
				"params": []map[string]string{
					{"name": "sendkey", "location": "body", "required": "true", "description": "发送密钥"},
					{"name": "msg", "location": "body", "required": "true", "description": "文本内容"},
					{"name": "msg_type", "location": "body", "required": "false", "description": "可不传，传入时固定为 text"},
					{"name": "image", "location": "body", "required": "true", "description": "图片 base64 字符串，也兼容旧字段 image_base62 / image_data"},
					{"name": "filename", "location": "body", "required": "false", "description": "图片文件名，未传时使用默认名"},
				},
			},
		},
		"verification_routes": []map[string]interface{}{
			{
				"title":       "企业微信回调 URL 验证",
				"method":      "GET",
				"path":        origin + "/" + callbackRoute,
				"description": "企业微信配置回调地址时使用。",
				"params": []map[string]string{
					{"name": "msg_signature", "location": "query", "required": "true", "description": "消息签名"},
					{"name": "timestamp", "location": "query", "required": "true", "description": "时间戳"},
					{"name": "nonce", "location": "query", "required": "true", "description": "随机串"},
					{"name": "echostr", "location": "query", "required": "true", "description": "企业微信下发的随机字符串"},
				},
			},
			{
				"title":       "企业微信回调消息接收",
				"method":      "POST",
				"path":        origin + "/" + callbackRoute,
				"description": "当前项目保留该接口入口，后续可扩展对回调消息的处理。",
				"params": []map[string]string{
					{"name": "msg_signature", "location": "query", "required": "true", "description": "消息签名"},
					{"name": "timestamp", "location": "query", "required": "true", "description": "时间戳"},
					{"name": "nonce", "location": "query", "required": "true", "description": "随机串"},
					{"name": "body", "location": "body", "required": "true", "description": "企业微信 XML 加密消息体"},
				},
			},
		},
	})
}

func adminExecuteGetTextHandler(res http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		writeJSONError(res, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	input, err := parseRequest(req)
	if err != nil {
		writeJSONError(res, http.StatusBadRequest, err.Error())
		return
	}
	if strings.TrimSpace(input.Payload.Msg) == "" {
		writeJSONError(res, http.StatusBadRequest, "msg is required")
		return
	}

	cfg, err := resolveAdminTargetConfig(input.Payload.RouteSuffix)
	if err != nil {
		writeJSONError(res, http.StatusBadRequest, err.Error())
		return
	}

	result, err := SendTextMessage(cfg, "text", input.Payload.Msg)
	if err != nil {
		writeJSONError(res, http.StatusBadGateway, err.Error())
		return
	}

	path := "/" + cfg.RouteSuffix
	writeJSON(res, http.StatusOK, map[string]interface{}{
		"request": map[string]interface{}{
			"route_suffix": cfg.RouteSuffix,
			"method":       "GET",
			"path":         path + "?sendkey=<SENDKEY>&msg=" + url.QueryEscape(input.Payload.Msg) + "&msg_type=text",
			"template":     "GET " + path + "?sendkey=<SENDKEY>&msg=" + url.QueryEscape(input.Payload.Msg) + "&msg_type=text",
		},
		"result": result,
	})
}

func adminExecutePostMessageHandler(res http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		writeJSONError(res, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	input, err := parseRequest(req)
	if err != nil {
		writeJSONError(res, http.StatusBadRequest, err.Error())
		return
	}

	msgType := strings.TrimSpace(input.Payload.MsgType)
	if msgType == "" {
		msgType = "text"
	}
	if msgType != "text" && msgType != "markdown" {
		writeJSONError(res, http.StatusBadRequest, "msg_type must be text or markdown")
		return
	}
	if strings.TrimSpace(input.Payload.Msg) == "" {
		writeJSONError(res, http.StatusBadRequest, "msg is required")
		return
	}

	cfg, err := resolveAdminTargetConfig(input.Payload.RouteSuffix)
	if err != nil {
		writeJSONError(res, http.StatusBadRequest, err.Error())
		return
	}

	result, err := SendTextMessage(cfg, msgType, input.Payload.Msg)
	if err != nil {
		writeJSONError(res, http.StatusBadGateway, err.Error())
		return
	}

	writeJSON(res, http.StatusOK, map[string]interface{}{
		"request": map[string]interface{}{
			"route_suffix": cfg.RouteSuffix,
			"method":       "POST",
			"path":         "/" + cfg.RouteSuffix,
			"body": map[string]interface{}{
				"sendkey":  "<SENDKEY>",
				"msg":      input.Payload.Msg,
				"msg_type": msgType,
			},
		},
		"result": result,
	})
}

func adminExecuteImageHandler(res http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		writeJSONError(res, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	input, err := parseRequest(req)
	if err != nil {
		writeJSONError(res, http.StatusBadRequest, err.Error())
		return
	}
	if input.Media == nil {
		writeJSONError(res, http.StatusBadRequest, "media file is required")
		return
	}

	cfg, err := resolveAdminTargetConfig(input.Payload.RouteSuffix)
	if err != nil {
		writeJSONError(res, http.StatusBadRequest, err.Error())
		return
	}

	result, err := SendImageMessage(cfg, input.Media.Filename, input.Media.Data)
	if err != nil {
		writeJSONError(res, http.StatusBadGateway, err.Error())
		return
	}

	writeJSON(res, http.StatusOK, map[string]interface{}{
		"request": map[string]interface{}{
			"route_suffix": cfg.RouteSuffix,
			"method":       "POST",
			"path":         "/" + cfg.RouteSuffix,
			"form": []map[string]string{
				{"name": "sendkey", "value": "<SENDKEY>"},
				{"name": "msg_type", "value": "image"},
				{"name": "media", "value": input.Media.Filename},
			},
		},
		"result": result,
	})
}

func adminExecuteTextImageHandler(res http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		writeJSONError(res, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	input, err := parseRequest(req)
	if err != nil {
		writeJSONError(res, http.StatusBadRequest, err.Error())
		return
	}
	if strings.TrimSpace(input.Payload.Msg) == "" {
		writeJSONError(res, http.StatusBadRequest, "msg is required")
		return
	}
	if msgType := strings.TrimSpace(input.Payload.MsgType); msgType != "" && msgType != "text" {
		writeJSONError(res, http.StatusBadRequest, "msg_type must be text when using image together with /<route>")
		return
	}
	if strings.TrimSpace(input.Payload.Image) == "" {
		writeJSONError(res, http.StatusBadRequest, "image is required")
		return
	}

	cfg, err := resolveAdminTargetConfig(input.Payload.RouteSuffix)
	if err != nil {
		writeJSONError(res, http.StatusBadRequest, err.Error())
		return
	}

	result, err := SendTextAndImageMessage(cfg, input.Payload.Msg, input.Payload.Image, input.Payload.Filename)
	if err != nil {
		writeJSONError(res, http.StatusBadGateway, err.Error())
		return
	}

	writeJSON(res, http.StatusOK, map[string]interface{}{
		"request": map[string]interface{}{
			"route_suffix": cfg.RouteSuffix,
			"method":       "POST",
			"path":         "/" + cfg.RouteSuffix,
			"body": map[string]interface{}{
				"sendkey":  "<SENDKEY>",
				"msg":      input.Payload.Msg,
				"msg_type": "text",
				"image":    "<BASE64_IMAGE_DATA>",
				"filename": input.Payload.Filename,
			},
		},
		"result": result,
	})
}

func decodeAdminLogin(req *http.Request, target *adminLoginRequest) error {
	if err := jsonNewDecoder(req.Body).Decode(target); err != nil {
		return fmt.Errorf("failed to parse request")
	}
	return nil
}

func requestOrigin(req *http.Request) string {
	scheme := "http"
	if req.TLS != nil {
		scheme = "https"
	}
	if forwarded := strings.TrimSpace(req.Header.Get("X-Forwarded-Proto")); forwarded != "" {
		scheme = forwarded
	}
	return scheme + "://" + req.Host
}

func adminDistAvailable() bool {
	indexPath := filepath.Join(FrontendDistDir, "index.html")
	info, err := os.Stat(indexPath)
	return err == nil && !info.IsDir()
}

type adminUIHandler struct {
	distDir string
}

func newAdminUIHandler(distDir string) http.Handler {
	return &adminUIHandler{distDir: distDir}
}

func (h *adminUIHandler) ServeHTTP(res http.ResponseWriter, req *http.Request) {
	if !adminDistAvailable() {
		writeJSONError(res, http.StatusServiceUnavailable, fmt.Sprintf("frontend dist not found in %s", h.distDir))
		return
	}

	relativePath := strings.TrimPrefix(req.URL.Path, "/admin")
	if relativePath == "" || relativePath == "/" {
		http.ServeFile(res, req, filepath.Join(h.distDir, "index.html"))
		return
	}

	cleaned := strings.TrimPrefix(filepath.Clean("/"+relativePath), "/")
	targetPath := filepath.Join(h.distDir, cleaned)
	if info, err := os.Stat(targetPath); err == nil && !info.IsDir() {
		http.ServeFile(res, req, targetPath)
		return
	}

	http.ServeFile(res, req, filepath.Join(h.distDir, "index.html"))
}

func jsonNewDecoder(reader io.Reader) *json.Decoder {
	decoder := json.NewDecoder(reader)
	decoder.DisallowUnknownFields()
	return decoder
}
