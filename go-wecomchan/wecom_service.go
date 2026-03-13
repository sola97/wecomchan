package main

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math/big"
	"mime/multipart"
	"net"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	redisTokenKeyPrefix = "access_token"
	tokenExpired        = 42001
	maxUploadSize       = 2 << 20
	base62Alphabet      = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
)

var (
	getTokenAPI    = "https://qyapi.weixin.qq.com/cgi-bin/gettoken?corpid=%s&corpsecret=%s"
	sendMessageAPI = "https://qyapi.weixin.qq.com/cgi-bin/message/send?access_token=%s"
	uploadMediaAPI = "https://qyapi.weixin.qq.com/cgi-bin/media/upload?access_token=%s&type=%s"
)

type Msg struct {
	Content string `json:"content"`
}

type Markdown struct {
	Content string `json:"content"`
}

type Pic struct {
	MediaId string `json:"media_id"`
}

type JsonData struct {
	ToUser                 string   `json:"touser"`
	AgentId                string   `json:"agentid"`
	MsgType                string   `json:"msgtype"`
	DuplicateCheckInterval int      `json:"duplicate_check_interval"`
	Text                   Msg      `json:"text,omitempty"`
	Image                  Pic      `json:"image,omitempty"`
	Markdown               Markdown `json:"markdown,omitempty"`
}

type tokenResponse struct {
	ErrCode     int    `json:"errcode"`
	ErrMsg      string `json:"errmsg"`
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
}

type wecomResponse struct {
	ErrCode int    `json:"errcode"`
	ErrMsg  string `json:"errmsg"`
}

type uploadResponse struct {
	ErrCode   int    `json:"errcode"`
	ErrMsg    string `json:"errmsg"`
	Type      string `json:"type"`
	MediaID   string `json:"media_id"`
	CreatedAt string `json:"created_at"`
}

type textImageSendResponse struct {
	TextResult  json.RawMessage `json:"text_result,omitempty"`
	ImageResult json.RawMessage `json:"image_result,omitempty"`
}

var errRedisNil = errors.New("redis nil")

func botRedisEnabled(cfg BotConfig) bool {
	redisCfg := currentRedisConfig()
	return redisCfg.Enabled && strings.TrimSpace(redisCfg.Addr) != ""
}

func tokenCacheKey(cfg BotConfig) string {
	return fmt.Sprintf("%s:%s:%s:%s", redisTokenKeyPrefix, cfg.RouteSuffix, cfg.WecomCID, cfg.WecomAID)
}

func currentRedisConfig() RedisConfig {
	if appState == nil || appState.BotConfigs == nil {
		return RedisConfig{}
	}
	return appState.BotConfigs.RedisConfig()
}

func redisDo(cfg BotConfig, args ...string) (string, error) {
	redisCfg := currentRedisConfig()
	conn, err := net.DialTimeout("tcp", redisCfg.Addr, 3*time.Second)
	if err != nil {
		return "", fmt.Errorf("connect redis: %w", err)
	}
	defer conn.Close()

	if err := conn.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
		return "", fmt.Errorf("set redis deadline: %w", err)
	}

	reader := bufio.NewReader(conn)
	if redisCfg.Password != "" {
		if err := writeRedisCommand(conn, "AUTH", redisCfg.Password); err != nil {
			return "", err
		}
		if _, err := readRedisResponse(reader); err != nil {
			return "", err
		}
	}

	if err := writeRedisCommand(conn, args...); err != nil {
		return "", err
	}
	return readRedisResponse(reader)
}

func writeRedisCommand(writer io.Writer, args ...string) error {
	if _, err := fmt.Fprintf(writer, "*%d\r\n", len(args)); err != nil {
		return fmt.Errorf("write redis array header: %w", err)
	}
	for _, arg := range args {
		if _, err := fmt.Fprintf(writer, "$%d\r\n%s\r\n", len(arg), arg); err != nil {
			return fmt.Errorf("write redis argument: %w", err)
		}
	}
	return nil
}

func readRedisResponse(reader *bufio.Reader) (string, error) {
	prefix, err := reader.ReadByte()
	if err != nil {
		return "", fmt.Errorf("read redis response prefix: %w", err)
	}

	line, err := reader.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("read redis response line: %w", err)
	}
	line = strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r")

	switch prefix {
	case '+', ':':
		return line, nil
	case '-':
		return "", fmt.Errorf("redis error: %s", line)
	case '$':
		size, err := strconv.Atoi(line)
		if err != nil {
			return "", fmt.Errorf("parse redis bulk length: %w", err)
		}
		if size == -1 {
			return "", errRedisNil
		}
		buf := make([]byte, size+2)
		if _, err := io.ReadFull(reader, buf); err != nil {
			return "", fmt.Errorf("read redis bulk body: %w", err)
		}
		return string(buf[:size]), nil
	default:
		return "", fmt.Errorf("unsupported redis response type: %q", prefix)
	}
}

func redisSet(cfg BotConfig, key, value string, ttl time.Duration) error {
	seconds := int(ttl.Seconds())
	if seconds <= 0 {
		seconds = 1
	}
	_, err := redisDo(cfg, "SET", key, value, "EX", strconv.Itoa(seconds))
	return err
}

func redisGet(cfg BotConfig, key string) (string, error) {
	return redisDo(cfg, "GET", key)
}

func redisDel(cfg BotConfig, key string) error {
	_, err := redisDo(cfg, "DEL", key)
	return err
}

func invalidateTokenCache(cfg BotConfig) {
	if !botRedisEnabled(cfg) {
		return
	}

	if err := redisDel(cfg, tokenCacheKey(cfg)); err != nil && !errors.Is(err, errRedisNil) {
		log.Printf("failed to delete redis token cache for %s: %v", cfg.RouteSuffix, err)
	}
}

func GetRemoteToken(cfg BotConfig) (string, error) {
	getTokenURL := fmt.Sprintf(getTokenAPI, cfg.WecomCID, cfg.WecomSecret)
	resp, err := httpClient.Get(getTokenURL)
	if err != nil {
		return "", fmt.Errorf("request access token: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read access token response: %w", err)
	}

	var tokenResp tokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return "", fmt.Errorf("decode access token response: %w", err)
	}
	if tokenResp.ErrCode != 0 {
		return "", fmt.Errorf("wecom get token failed: %s (%d)", tokenResp.ErrMsg, tokenResp.ErrCode)
	}
	if tokenResp.AccessToken == "" {
		return "", fmt.Errorf("wecom get token returned empty access_token")
	}

	if botRedisEnabled(cfg) {
		expire := 7000 * time.Second
		if tokenResp.ExpiresIn > 120 {
			expire = time.Duration(tokenResp.ExpiresIn-60) * time.Second
		}
		if err := redisSet(cfg, tokenCacheKey(cfg), tokenResp.AccessToken, expire); err != nil {
			log.Printf("failed to cache access token in redis for %s: %v", cfg.RouteSuffix, err)
		}
	}

	return tokenResp.AccessToken, nil
}

func GetAccessToken(cfg BotConfig) (string, error) {
	if botRedisEnabled(cfg) {
		value, err := redisGet(cfg, tokenCacheKey(cfg))
		if err == nil && value != "" {
			return value, nil
		}
		if err != nil && !errors.Is(err, errRedisNil) {
			log.Printf("failed to get access token from redis for %s: %v", cfg.RouteSuffix, err)
		}
	}
	return GetRemoteToken(cfg)
}

func InitJsonData(cfg BotConfig, msgType string) JsonData {
	return JsonData{
		ToUser:                 cfg.WecomToUID,
		AgentId:                cfg.WecomAID,
		MsgType:                msgType,
		DuplicateCheckInterval: 600,
	}
}

func postMessage(postData JsonData, accessToken string) (json.RawMessage, wecomResponse, error) {
	postJSON, err := json.Marshal(postData)
	if err != nil {
		return nil, wecomResponse{}, fmt.Errorf("marshal send payload: %w", err)
	}

	sendMessageURL := fmt.Sprintf(sendMessageAPI, accessToken)
	req, err := http.NewRequest(http.MethodPost, sendMessageURL, bytes.NewReader(postJSON))
	if err != nil {
		return nil, wecomResponse{}, fmt.Errorf("build send request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, wecomResponse{}, fmt.Errorf("send wecom message: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, wecomResponse{}, fmt.Errorf("read send response: %w", err)
	}

	var parsed wecomResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, wecomResponse{}, fmt.Errorf("decode send response: %w", err)
	}
	return json.RawMessage(body), parsed, nil
}

func uploadMedia(msgType, filename string, data []byte, accessToken string) (uploadResponse, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	part, err := writer.CreateFormFile("media", normalizeFilename(filename, "upload-image"))
	if err != nil {
		return uploadResponse{}, fmt.Errorf("create upload form file: %w", err)
	}
	if _, err := part.Write(data); err != nil {
		return uploadResponse{}, fmt.Errorf("write upload file content: %w", err)
	}
	if err := writer.Close(); err != nil {
		return uploadResponse{}, fmt.Errorf("close upload form writer: %w", err)
	}

	uploadMediaURL := fmt.Sprintf(uploadMediaAPI, accessToken, msgType)
	req, err := http.NewRequest(http.MethodPost, uploadMediaURL, &body)
	if err != nil {
		return uploadResponse{}, fmt.Errorf("build upload request: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := httpClient.Do(req)
	if err != nil {
		return uploadResponse{}, fmt.Errorf("upload media: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return uploadResponse{}, fmt.Errorf("read upload response: %w", err)
	}

	var parsed uploadResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return uploadResponse{}, fmt.Errorf("decode upload response: %w", err)
	}
	return parsed, nil
}

func sendMessageWithRetry(cfg BotConfig, postData JsonData) (json.RawMessage, error) {
	accessToken, err := GetAccessToken(cfg)
	if err != nil {
		return nil, err
	}

	var lastRaw json.RawMessage
	var parsed wecomResponse
	for attempt := 0; attempt < 4; attempt++ {
		lastRaw, parsed, err = postMessage(postData, accessToken)
		if err != nil {
			return nil, err
		}
		if parsed.ErrCode != tokenExpired {
			return lastRaw, nil
		}
		invalidateTokenCache(cfg)
		accessToken, err = GetAccessToken(cfg)
		if err != nil {
			return nil, err
		}
	}
	return lastRaw, nil
}

func uploadMediaWithRetry(cfg BotConfig, msgType, filename string, data []byte) (uploadResponse, error) {
	accessToken, err := GetAccessToken(cfg)
	if err != nil {
		return uploadResponse{}, err
	}

	var parsed uploadResponse
	for attempt := 0; attempt < 4; attempt++ {
		parsed, err = uploadMedia(msgType, filename, data, accessToken)
		if err != nil {
			return uploadResponse{}, err
		}
		if parsed.ErrCode != tokenExpired {
			return parsed, nil
		}
		invalidateTokenCache(cfg)
		accessToken, err = GetAccessToken(cfg)
		if err != nil {
			return uploadResponse{}, err
		}
	}
	return parsed, nil
}

func normalizeFilename(filename, fallback string) string {
	clean := strings.TrimSpace(filepath.Base(filename))
	if clean == "." || clean == "/" || clean == "" {
		return fallback
	}
	return clean
}

func SendTextMessage(cfg BotConfig, msgType, content string) (json.RawMessage, error) {
	postData := InitJsonData(cfg, msgType)
	switch msgType {
	case "markdown":
		postData.Markdown = Markdown{Content: content}
	case "text":
		postData.Text = Msg{Content: content}
	default:
		return nil, fmt.Errorf("unsupported text message type: %s", msgType)
	}
	result, err := sendMessageWithRetry(cfg, postData)
	if err != nil {
		return nil, err
	}
	recordMessageLog(cfg, msgType, content, "", 0)
	return result, nil
}

func SendImageMessage(cfg BotConfig, filename string, data []byte) (json.RawMessage, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("image data is required")
	}
	if len(data) > maxUploadSize {
		return nil, fmt.Errorf("image exceeds 2MB limit")
	}

	uploadResp, err := uploadMediaWithRetry(cfg, "image", filename, data)
	if err != nil {
		return nil, err
	}
	if uploadResp.ErrCode != 0 {
		return nil, fmt.Errorf("wecom upload failed: %s (%d)", uploadResp.ErrMsg, uploadResp.ErrCode)
	}

	postData := InitJsonData(cfg, "image")
	postData.Image = Pic{MediaId: uploadResp.MediaID}
	result, err := sendMessageWithRetry(cfg, postData)
	if err != nil {
		return nil, err
	}
	recordMessageLog(cfg, "image", "", filename, len(data))
	return result, nil
}

func SendTextAndImageMessage(cfg BotConfig, text, imagePayload, filename string) (textImageSendResponse, error) {
	imageBytes, err := DecodeImagePayload(strings.TrimSpace(imagePayload))
	if err != nil {
		return textImageSendResponse{}, err
	}

	textResult, err := SendTextMessage(cfg, "text", text)
	if err != nil {
		return textImageSendResponse{}, err
	}

	imageResult, err := SendImageMessage(cfg, filename, imageBytes)
	if err != nil {
		return textImageSendResponse{TextResult: textResult}, err
	}

	return textImageSendResponse{
		TextResult:  textResult,
		ImageResult: imageResult,
	}, nil
}

func DecodeImagePayload(value string) ([]byte, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil, fmt.Errorf("image is required")
	}

	if strings.HasPrefix(trimmed, "data:") {
		parts := strings.SplitN(trimmed, ",", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid image data URL")
		}
		decoded, err := decodeBase64Payload(parts[1])
		if err != nil {
			return nil, err
		}
		if !isLikelyImageBytes(decoded) {
			return nil, fmt.Errorf("decoded data URL is not a supported image")
		}
		return decoded, nil
	}

	if decoded, err := decodeBase64Payload(trimmed); err == nil && isLikelyImageBytes(decoded) {
		return decoded, nil
	}
	if decoded, err := decodeRawBase62(trimmed); err == nil && isLikelyImageBytes(decoded) {
		return decoded, nil
	}

	decoded, err := decodeBase64Payload(trimmed)
	if err == nil {
		return decoded, nil
	}
	return decodeRawBase62(trimmed)
}

func decodeRawBase62(value string) ([]byte, error) {
	indexes := make(map[rune]int, len(base62Alphabet))
	for idx, char := range base62Alphabet {
		indexes[char] = idx
	}

	number := big.NewInt(0)
	base := big.NewInt(62)
	for _, char := range value {
		idx, ok := indexes[char]
		if !ok {
			return nil, fmt.Errorf("invalid base62 character: %q", char)
		}
		number.Mul(number, base)
		number.Add(number, big.NewInt(int64(idx)))
	}

	decoded := number.Bytes()
	leadingZeroBytes := 0
	for leadingZeroBytes < len(value) && value[leadingZeroBytes] == base62Alphabet[0] {
		leadingZeroBytes++
	}
	if leadingZeroBytes > 0 {
		decoded = append(make([]byte, leadingZeroBytes), decoded...)
	}
	if len(decoded) == 0 {
		return nil, fmt.Errorf("decoded image is empty")
	}
	return decoded, nil
}

func decodeBase64Payload(value string) ([]byte, error) {
	encodings := []*base64.Encoding{
		base64.StdEncoding,
		base64.RawStdEncoding,
		base64.URLEncoding,
		base64.RawURLEncoding,
	}

	var lastErr error
	for _, encoding := range encodings {
		decoded, err := encoding.DecodeString(value)
		if err == nil {
			return decoded, nil
		}
		lastErr = err
	}

	if mod := len(value) % 4; mod != 0 {
		padded := value + strings.Repeat("=", 4-mod)
		decoded, err := base64.StdEncoding.DecodeString(padded)
		if err == nil {
			return decoded, nil
		}
		lastErr = err
	}

	return nil, lastErr
}

func isLikelyImageBytes(data []byte) bool {
	if len(data) == 0 {
		return false
	}
	contentType := http.DetectContentType(data)
	return strings.HasPrefix(contentType, "image/")
}
