package main

import (
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"strings"
)

type requestPayload struct {
	RouteSuffix   string `json:"route_suffix"`
	SendKey       string `json:"sendkey"`
	Msg           string `json:"msg"`
	MsgType       string `json:"msg_type"`
	Image         string `json:"image"`
	ImageBase62   string `json:"image_base62"`
	ImageData     string `json:"image_data"`
	Filename      string `json:"filename"`
	ImageFilename string `json:"image_filename"`
}

type uploadedFile struct {
	Filename string
	Data     []byte
}

type parsedRequest struct {
	Payload requestPayload
	Media   *uploadedFile
}

func hasContentType(req *http.Request, want string) bool {
	contentType := req.Header.Get("Content-Type")
	if contentType == "" {
		return want == ContentTypeBinary
	}
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return false
	}
	return mediaType == want
}

func overrideIfNotEmpty(dst *string, value string) {
	if value != "" {
		*dst = value
	}
}

func fillIfEmpty(dst *string, value string) {
	if *dst == "" && value != "" {
		*dst = value
	}
}

func parseRequest(req *http.Request) (parsedRequest, error) {
	result := parsedRequest{
		Payload: requestPayload{
			RouteSuffix:   req.URL.Query().Get("route_suffix"),
			SendKey:       req.URL.Query().Get("sendkey"),
			Msg:           req.URL.Query().Get("msg"),
			MsgType:       req.URL.Query().Get("msg_type"),
			Image:         req.URL.Query().Get("image"),
			ImageBase62:   req.URL.Query().Get("image_base62"),
			ImageData:     req.URL.Query().Get("image_data"),
			Filename:      req.URL.Query().Get("filename"),
			ImageFilename: req.URL.Query().Get("image_filename"),
		},
	}

	if req.Method != http.MethodPost {
		fillIfEmpty(&result.Payload.SendKey, req.Header.Get("sendkey"))
		fillIfEmpty(&result.Payload.MsgType, req.Header.Get("msgtype"))
		fillIfEmpty(&result.Payload.RouteSuffix, req.Header.Get("route_suffix"))
		return result, nil
	}

	switch {
	case hasContentType(req, ContentTypeJSON):
		var body requestPayload
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			return parsedRequest{}, fmt.Errorf("invalid json body")
		}
		overrideIfNotEmpty(&result.Payload.RouteSuffix, body.RouteSuffix)
		overrideIfNotEmpty(&result.Payload.SendKey, body.SendKey)
		overrideIfNotEmpty(&result.Payload.Msg, body.Msg)
		overrideIfNotEmpty(&result.Payload.MsgType, body.MsgType)
		overrideIfNotEmpty(&result.Payload.Image, body.Image)
		overrideIfNotEmpty(&result.Payload.ImageBase62, body.ImageBase62)
		overrideIfNotEmpty(&result.Payload.ImageData, body.ImageData)
		overrideIfNotEmpty(&result.Payload.Filename, body.Filename)
		overrideIfNotEmpty(&result.Payload.ImageFilename, body.ImageFilename)
	case hasContentType(req, ContentTypeFormData):
		if err := req.ParseMultipartForm(maxUploadSize); err != nil {
			return parsedRequest{}, fmt.Errorf("failed to parse multipart form")
		}
		overrideIfNotEmpty(&result.Payload.RouteSuffix, req.FormValue("route_suffix"))
		overrideIfNotEmpty(&result.Payload.SendKey, req.FormValue("sendkey"))
		overrideIfNotEmpty(&result.Payload.Msg, req.FormValue("msg"))
		overrideIfNotEmpty(&result.Payload.MsgType, req.FormValue("msg_type"))
		overrideIfNotEmpty(&result.Payload.Image, req.FormValue("image"))
		overrideIfNotEmpty(&result.Payload.ImageBase62, req.FormValue("image_base62"))
		overrideIfNotEmpty(&result.Payload.ImageData, req.FormValue("image_data"))
		overrideIfNotEmpty(&result.Payload.Filename, req.FormValue("filename"))
		overrideIfNotEmpty(&result.Payload.ImageFilename, req.FormValue("image_filename"))

		file, header, err := req.FormFile("media")
		if err == nil {
			defer file.Close()
			data, readErr := io.ReadAll(io.LimitReader(file, maxUploadSize+1))
			if readErr != nil {
				return parsedRequest{}, fmt.Errorf("failed to read uploaded image")
			}
			if len(data) > maxUploadSize {
				return parsedRequest{}, fmt.Errorf("image exceeds 2MB limit")
			}
			result.Media = &uploadedFile{
				Filename: header.Filename,
				Data:     data,
			}
		}
	case hasContentType(req, ContentTypeForm):
		if err := req.ParseForm(); err != nil {
			return parsedRequest{}, fmt.Errorf("failed to parse form body")
		}
		overrideIfNotEmpty(&result.Payload.RouteSuffix, req.FormValue("route_suffix"))
		overrideIfNotEmpty(&result.Payload.SendKey, req.FormValue("sendkey"))
		overrideIfNotEmpty(&result.Payload.Msg, req.FormValue("msg"))
		overrideIfNotEmpty(&result.Payload.MsgType, req.FormValue("msg_type"))
		overrideIfNotEmpty(&result.Payload.Image, req.FormValue("image"))
		overrideIfNotEmpty(&result.Payload.ImageBase62, req.FormValue("image_base62"))
		overrideIfNotEmpty(&result.Payload.ImageData, req.FormValue("image_data"))
		overrideIfNotEmpty(&result.Payload.Filename, req.FormValue("filename"))
		overrideIfNotEmpty(&result.Payload.ImageFilename, req.FormValue("image_filename"))
	}

	fillIfEmpty(&result.Payload.RouteSuffix, req.Header.Get("route_suffix"))
	fillIfEmpty(&result.Payload.SendKey, req.Header.Get("sendkey"))
	fillIfEmpty(&result.Payload.MsgType, req.Header.Get("msgtype"))
	fillIfEmpty(&result.Payload.Image, result.Payload.ImageBase62)
	fillIfEmpty(&result.Payload.Image, result.Payload.ImageData)
	fillIfEmpty(&result.Payload.Filename, result.Payload.ImageFilename)

	return result, nil
}

func validateSendKey(cfg BotConfig, sendKey string) error {
	if strings.TrimSpace(sendKey) == "" {
		return fmt.Errorf("sendkey is required")
	}
	if subtle.ConstantTimeCompare([]byte(sendKey), []byte(cfg.SendKey)) != 1 {
		return fmt.Errorf("invalid sendkey")
	}
	return nil
}

func resolveBotConfig(routeSuffix string) (BotConfig, error) {
	if appState == nil || appState.BotConfigs == nil {
		return BotConfig{}, fmt.Errorf("bot config store is not initialized")
	}

	if routeSuffix != "" {
		cfg, ok := appState.BotConfigs.Get(routeSuffix)
		if !ok {
			return BotConfig{}, fmt.Errorf("route %q not found", normalizeRouteSuffix(routeSuffix))
		}
		return cfg, nil
	}

	cfg, ok := appState.BotConfigs.Default()
	if !ok {
		return BotConfig{}, fmt.Errorf("no bot configuration available")
	}
	return cfg, nil
}

func handleBotSend(res http.ResponseWriter, req *http.Request, cfg BotConfig) {
	if req.Method != http.MethodGet && req.Method != http.MethodPost {
		writeJSONError(res, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	input, err := parseRequest(req)
	if err != nil {
		writeJSONError(res, http.StatusBadRequest, err.Error())
		return
	}
	if err := validateSendKey(cfg, input.Payload.SendKey); err != nil {
		writeJSONError(res, http.StatusUnauthorized, err.Error())
		return
	}

	msgType := strings.TrimSpace(input.Payload.MsgType)
	if msgType == "" {
		msgType = "text"
	}

	var result interface{}
	switch msgType {
	case "text", "markdown":
		if strings.TrimSpace(input.Payload.Msg) == "" {
			writeJSONError(res, http.StatusBadRequest, "msg is required")
			return
		}
		if strings.TrimSpace(input.Payload.Image) != "" {
			if msgType != "text" {
				writeJSONError(res, http.StatusBadRequest, "image can only be used together with text messages")
				return
			}
			result, err = SendTextAndImageMessage(cfg, input.Payload.Msg, input.Payload.Image, input.Payload.Filename)
		} else {
			result, err = SendTextMessage(cfg, msgType, input.Payload.Msg)
		}
	case "image":
		if input.Media == nil {
			writeJSONError(res, http.StatusBadRequest, "media file is required for image messages")
			return
		}
		result, err = SendImageMessage(cfg, input.Media.Filename, input.Media.Data)
	default:
		writeJSONError(res, http.StatusBadRequest, "msg_type must be text, markdown, or image")
		return
	}
	if err != nil {
		writeJSONError(res, http.StatusBadGateway, err.Error())
		return
	}

	writeJSON(res, http.StatusOK, result)
}

func handleBotLegacyTextImage(res http.ResponseWriter, req *http.Request, cfg BotConfig) {
	if req.Method != http.MethodPost {
		writeJSONError(res, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	input, err := parseRequest(req)
	if err != nil {
		writeJSONError(res, http.StatusBadRequest, err.Error())
		return
	}
	if err := validateSendKey(cfg, input.Payload.SendKey); err != nil {
		writeJSONError(res, http.StatusUnauthorized, err.Error())
		return
	}
	if strings.TrimSpace(input.Payload.Msg) == "" {
		writeJSONError(res, http.StatusBadRequest, "msg is required")
		return
	}
	if strings.TrimSpace(input.Payload.Image) == "" {
		writeJSONError(res, http.StatusBadRequest, "image is required")
		return
	}

	result, err := SendTextAndImageMessage(cfg, input.Payload.Msg, input.Payload.Image, input.Payload.Filename)
	if err != nil {
		writeJSONError(res, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(res, http.StatusOK, result)
}

func legacyCallbackHandler(res http.ResponseWriter, req *http.Request) {
	cfg, err := resolveBotConfig("")
	if err != nil {
		writeJSONError(res, http.StatusNotFound, err.Error())
		return
	}
	HandleBotCallback(res, req, cfg)
}

func dynamicRouteHandler(res http.ResponseWriter, req *http.Request) {
	if req.URL.Path == "/" {
		rootHandler(res, req)
		return
	}

	trimmedPath := strings.Trim(req.URL.Path, "/")
	if trimmedPath == "" {
		rootHandler(res, req)
		return
	}

	segments := strings.Split(trimmedPath, "/")
	switch len(segments) {
	case 1:
		cfg, err := resolveBotConfig(segments[0])
		if err != nil {
			http.NotFound(res, req)
			return
		}
		handleBotSend(res, req, cfg)
	case 2:
		cfg, err := resolveBotConfig(segments[0])
		if err != nil {
			http.NotFound(res, req)
			return
		}
		switch segments[1] {
		case "callback":
			HandleBotCallback(res, req, cfg)
		case "base62":
			handleBotLegacyTextImage(res, req, cfg)
		default:
			http.NotFound(res, req)
		}
	default:
		http.NotFound(res, req)
	}
}

func rootHandler(res http.ResponseWriter, req *http.Request) {
	if req.URL.Path != "/" {
		http.NotFound(res, req)
		return
	}

	message := fmt.Sprintf(
		"Wecomchan is running. Admin UI: /admin/ . Configs loaded: %d\n",
		appState.BotConfigs.Count(),
	)
	res.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = res.Write([]byte(message))
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	botConfigs, err := NewBotConfigStore(BotConfigPath)
	if err != nil {
		log.Fatal(err)
	}
	appState = &AppState{
		BotConfigs:  botConfigs,
		MessageLogs: NewMessageLogStore(MessageLogPath),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/callback", legacyCallbackHandler)
	registerAdminRoutes(mux)
	mux.HandleFunc("/", dynamicRouteHandler)

	log.Printf("starting wecomchan on %s", ListenAddr)
	log.Fatal(http.ListenAndServe(ListenAddr, mux))
}
