package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"net/http"
	"sort"
	"strings"
)

type QueryParams struct {
	MsgSignature string `form:"msg_signature"`
	TimeStamp    string `form:"timestamp"`
	Nonce        string `form:"nonce"`
	EchoStr      string `form:"echostr"`
}

func HandleBotCallback(res http.ResponseWriter, req *http.Request, cfg BotConfig) {
	if req.Method != http.MethodGet {
		writeJSONError(res, http.StatusNotImplemented, "only GET callback verification is implemented")
		return
	}
	if cfg.CallbackToken == "" || cfg.CallbackAESKey == "" {
		writeJSONError(res, http.StatusBadRequest, "callback_token and callback_aes_key are required for callback verification")
		return
	}

	q := QueryParams{
		MsgSignature: req.URL.Query().Get("msg_signature"),
		TimeStamp:    req.URL.Query().Get("timestamp"),
		Nonce:        req.URL.Query().Get("nonce"),
		EchoStr:      req.URL.Query().Get("echostr"),
	}

	if q.MsgSignature == "" || q.TimeStamp == "" || q.Nonce == "" || q.EchoStr == "" {
		writeJSONError(res, http.StatusBadRequest, "msg_signature, timestamp, nonce, and echostr are required")
		return
	}

	expectedSignature := calculateCallbackSignature(cfg.CallbackToken, q.TimeStamp, q.Nonce, q.EchoStr)
	if !strings.EqualFold(expectedSignature, q.MsgSignature) {
		writeJSONError(res, http.StatusUnauthorized, "invalid callback signature")
		return
	}

	echoText, err := decryptCallbackEcho(cfg, q.EchoStr)
	if err != nil {
		writeJSONError(res, http.StatusBadRequest, err.Error())
		return
	}

	res.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = res.Write([]byte(echoText))
}

func calculateCallbackSignature(token, timestamp, nonce, encrypted string) string {
	values := []string{token, timestamp, nonce, encrypted}
	sort.Strings(values)
	sum := sha1.Sum([]byte(strings.Join(values, "")))
	return fmt.Sprintf("%x", sum[:])
}

func decryptCallbackEcho(cfg BotConfig, encrypted string) (string, error) {
	aesKey, err := base64.StdEncoding.DecodeString(cfg.CallbackAESKey + "=")
	if err != nil {
		return "", fmt.Errorf("invalid callback_aes_key: %w", err)
	}
	if len(aesKey) != 32 {
		return "", fmt.Errorf("invalid callback_aes_key length")
	}

	cipherText, err := base64.StdEncoding.DecodeString(encrypted)
	if err != nil {
		return "", fmt.Errorf("decode echostr: %w", err)
	}
	if len(cipherText) == 0 || len(cipherText)%aes.BlockSize != 0 {
		return "", fmt.Errorf("invalid echostr payload length")
	}

	block, err := aes.NewCipher(aesKey)
	if err != nil {
		return "", fmt.Errorf("create aes cipher: %w", err)
	}

	plainText := make([]byte, len(cipherText))
	cipher.NewCBCDecrypter(block, aesKey[:aes.BlockSize]).CryptBlocks(plainText, cipherText)

	plainText, err = pkcs7Unpad(plainText)
	if err != nil {
		return "", err
	}
	if len(plainText) < 20 {
		return "", fmt.Errorf("invalid echostr plaintext length")
	}

	content := plainText[16:]
	messageLength := binary.BigEndian.Uint32(content[:4])
	if int(4+messageLength) > len(content) {
		return "", fmt.Errorf("invalid echostr message length")
	}

	message := string(content[4 : 4+messageLength])
	receiveID := string(content[4+messageLength:])
	if receiveID != "" && receiveID != cfg.WecomCID {
		return "", fmt.Errorf("callback receive id mismatch")
	}
	return message, nil
}

func pkcs7Unpad(value []byte) ([]byte, error) {
	if len(value) == 0 {
		return nil, fmt.Errorf("invalid empty plaintext")
	}

	padding := int(value[len(value)-1])
	if padding < 1 || padding > 32 || padding > len(value) {
		return nil, fmt.Errorf("invalid callback padding")
	}
	return value[:len(value)-padding], nil
}
