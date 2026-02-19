package telegram

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTranscribeAudio(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Error("missing auth header")
		}

		ct := r.Header.Get("Content-Type")
		if !strings.Contains(ct, "multipart/form-data") {
			t.Errorf("expected multipart form, got %q", ct)
		}

		if err := r.ParseMultipartForm(10 << 20); err != nil {
			t.Fatalf("parse form: %v", err)
		}

		model := r.FormValue("model")
		if model != "whisper-large-v3-turbo" {
			t.Errorf("model = %q", model)
		}

		format := r.FormValue("response_format")
		if format != "json" {
			t.Errorf("response_format = %q", format)
		}

		_, fh, err := r.FormFile("file")
		if err != nil {
			t.Fatalf("form file: %v", err)
		}
		if fh.Filename != "test.ogg" {
			t.Errorf("filename = %q", fh.Filename)
		}

		json.NewEncoder(w).Encode(whisperResponse{Text: "Hello, this is a test."})
	}))
	defer srv.Close()

	// Create a temp audio file
	tmpDir := t.TempDir()
	audioPath := filepath.Join(tmpDir, "test.ogg")
	os.WriteFile(audioPath, []byte("fake audio data"), 0o644)

	cfg := &VoiceConfig{
		WhisperURL:    srv.URL,
		WhisperAPIKey: "test-key",
	}

	text, err := transcribeAudio(context.Background(), cfg, audioPath)
	if err != nil {
		t.Fatalf("transcribeAudio: %v", err)
	}
	if text != "Hello, this is a test." {
		t.Errorf("text = %q", text)
	}
}

func TestTranscribeAudio_CustomModel(t *testing.T) {
	var gotModel string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.ParseMultipartForm(10 << 20)
		gotModel = r.FormValue("model")
		json.NewEncoder(w).Encode(whisperResponse{Text: "ok"})
	}))
	defer srv.Close()

	tmpDir := t.TempDir()
	audioPath := filepath.Join(tmpDir, "test.ogg")
	os.WriteFile(audioPath, []byte("audio"), 0o644)

	cfg := &VoiceConfig{
		WhisperURL:    srv.URL,
		WhisperAPIKey: "key",
		WhisperModel:  "whisper-1",
	}

	_, err := transcribeAudio(context.Background(), cfg, audioPath)
	if err != nil {
		t.Fatalf("transcribeAudio: %v", err)
	}
	if gotModel != "whisper-1" {
		t.Errorf("model = %q, want whisper-1", gotModel)
	}
}

func TestTranscribeAudio_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"error": "rate limited"}`))
	}))
	defer srv.Close()

	tmpDir := t.TempDir()
	audioPath := filepath.Join(tmpDir, "test.ogg")
	os.WriteFile(audioPath, []byte("audio"), 0o644)

	cfg := &VoiceConfig{
		WhisperURL:    srv.URL,
		WhisperAPIKey: "key",
	}

	_, err := transcribeAudio(context.Background(), cfg, audioPath)
	if err == nil {
		t.Fatal("expected error for 429 status")
	}
}

func TestDownloadFile(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("audio bytes"))
	}))
	defer srv.Close()

	data, err := downloadFile(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("downloadFile: %v", err)
	}
	if string(data) != "audio bytes" {
		t.Errorf("data = %q", string(data))
	}
}

func TestDownloadFile_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	_, err := downloadFile(context.Background(), srv.URL)
	if err == nil {
		t.Fatal("expected error for 404")
	}
}
