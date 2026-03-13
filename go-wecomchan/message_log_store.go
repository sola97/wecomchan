package main

import (
	"bufio"
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const defaultMessageLogLimit = 200

type MessageLogEntry struct {
	Timestamp      string `json:"timestamp"`
	RouteSuffix    string `json:"route_suffix"`
	DisplayName    string `json:"display_name"`
	MsgType        string `json:"msg_type"`
	ContentPreview string `json:"content_preview,omitempty"`
	Filename       string `json:"filename,omitempty"`
	SizeBytes      int    `json:"size_bytes,omitempty"`
}

type MessageLogStore struct {
	mu   sync.Mutex
	path string
}

func NewMessageLogStore(path string) *MessageLogStore {
	return &MessageLogStore{path: path}
}

func (s *MessageLogStore) Append(entry MessageLogEntry) error {
	if s == nil {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	entry.Timestamp = strings.TrimSpace(entry.Timestamp)
	if entry.Timestamp == "" {
		entry.Timestamp = time.Now().Format(time.RFC3339)
	}

	payload, err := json.Marshal(entry)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}

	file, err := os.OpenFile(s.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()

	if _, err := file.Write(append(payload, '\n')); err != nil {
		return err
	}
	return nil
}

func (s *MessageLogStore) ListRecent(within time.Duration, limit int) ([]MessageLogEntry, error) {
	if s == nil {
		return []MessageLogEntry{}, nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if limit <= 0 {
		limit = defaultMessageLogLimit
	}

	file, err := os.Open(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return []MessageLogEntry{}, nil
		}
		return nil, err
	}
	defer file.Close()

	cutoff := time.Now().Add(-within)
	entries := make([]MessageLogEntry, 0, limit)
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var entry MessageLogEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}

		parsedTime, err := time.Parse(time.RFC3339, entry.Timestamp)
		if err != nil {
			continue
		}
		if parsedTime.Before(cutoff) {
			continue
		}

		entries = append(entries, entry)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	for left, right := 0, len(entries)-1; left < right; left, right = left+1, right-1 {
		entries[left], entries[right] = entries[right], entries[left]
	}
	if len(entries) > limit {
		entries = entries[:limit]
	}
	return entries, nil
}

func truncateLogContent(value string, maxRunes int) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}

	runes := []rune(value)
	if len(runes) <= maxRunes {
		return value
	}
	return string(runes[:maxRunes]) + "..."
}

func recordMessageLog(cfg BotConfig, msgType, content, filename string, sizeBytes int) {
	if appState == nil || appState.MessageLogs == nil {
		return
	}

	entry := MessageLogEntry{
		RouteSuffix:    cfg.RouteSuffix,
		DisplayName:    cfg.DisplayName,
		MsgType:        strings.TrimSpace(msgType),
		ContentPreview: truncateLogContent(content, 180),
		Filename:       normalizeFilename(filename, ""),
		SizeBytes:      sizeBytes,
	}
	if err := appState.MessageLogs.Append(entry); err != nil {
		log.Printf("failed to append message log: %v", err)
	}
}
