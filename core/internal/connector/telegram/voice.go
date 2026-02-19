package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// VoiceConfig holds voice transcription settings.
type VoiceConfig struct {
	// WhisperURL is the Whisper API endpoint.
	// Supports OpenAI-compatible endpoints (OpenAI, Groq, etc.)
	// Default: https://api.groq.com/openai/v1/audio/transcriptions
	WhisperURL string
	// WhisperAPIKey is the API key for the Whisper service.
	WhisperAPIKey string
	// WhisperModel is the model to use (default: "whisper-large-v3-turbo").
	WhisperModel string
}

// transcribeVoice downloads and transcribes a Telegram voice message.
func (c *Connector) transcribeVoice(ctx context.Context, msg *tgbotapi.Message) (string, error) {
	if c.config.Voice == nil || c.config.Voice.WhisperAPIKey == "" {
		return "", fmt.Errorf("voice transcription not configured")
	}

	var fileID string
	if msg.Voice != nil {
		fileID = msg.Voice.FileID
	} else if msg.Audio != nil {
		fileID = msg.Audio.FileID
	} else {
		return "", fmt.Errorf("no voice or audio in message")
	}

	// Download the audio file from Telegram
	fileURL, err := c.bot.GetFileDirectURL(fileID)
	if err != nil {
		return "", fmt.Errorf("get file URL: %w", err)
	}

	audioData, err := downloadFile(ctx, fileURL)
	if err != nil {
		return "", fmt.Errorf("download audio: %w", err)
	}

	// Save to temp file (Whisper API requires a file upload)
	tmpDir := os.TempDir()
	tmpFile := filepath.Join(tmpDir, fmt.Sprintf("h1v3_voice_%d.ogg", time.Now().UnixNano()))
	if err := os.WriteFile(tmpFile, audioData, 0o600); err != nil {
		return "", fmt.Errorf("save temp audio: %w", err)
	}
	defer os.Remove(tmpFile)

	// Transcribe via Whisper API
	text, err := transcribeAudio(ctx, c.config.Voice, tmpFile)
	if err != nil {
		return "", fmt.Errorf("transcribe: %w", err)
	}

	return text, nil
}

func downloadFile(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download status %d", resp.StatusCode)
	}

	// Limit to 25MB (Telegram voice limit is 20MB)
	return io.ReadAll(io.LimitReader(resp.Body, 25<<20))
}

// transcribeAudio sends an audio file to a Whisper-compatible API.
func transcribeAudio(ctx context.Context, cfg *VoiceConfig, audioPath string) (string, error) {
	url := cfg.WhisperURL
	if url == "" {
		url = "https://api.groq.com/openai/v1/audio/transcriptions"
	}
	model := cfg.WhisperModel
	if model == "" {
		model = "whisper-large-v3-turbo"
	}

	// Build multipart form
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)

	// Add file
	fw, err := w.CreateFormFile("file", filepath.Base(audioPath))
	if err != nil {
		return "", err
	}
	f, err := os.Open(audioPath)
	if err != nil {
		return "", err
	}
	defer f.Close()
	if _, err := io.Copy(fw, f); err != nil {
		return "", err
	}

	// Add model field
	w.WriteField("model", model)
	w.WriteField("response_format", "json")
	w.Close()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, &buf)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+cfg.WhisperAPIKey)

	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("whisper API error (status %d): %s", resp.StatusCode, string(body))
	}

	var result whisperResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("parse whisper response: %w", err)
	}

	return result.Text, nil
}

type whisperResponse struct {
	Text string `json:"text"`
}
