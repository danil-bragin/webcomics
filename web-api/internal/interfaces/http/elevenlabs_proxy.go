package http

import (
	"encoding/json"
	"net/http"
	"os"
	"sync"
	"time"
)

// ElevenLabsVoice mirrors the shape returned by /v1/voices, but pares it down to
// what the UI actually needs (id, name, labels).
type ElevenLabsVoice struct {
	VoiceID     string            `json:"voice_id"`
	Name        string            `json:"name"`
	Category    string            `json:"category,omitempty"`
	Description string            `json:"description,omitempty"`
	PreviewURL  string            `json:"preview_url,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
}

type voicesCache struct {
	mu     sync.Mutex
	at     time.Time
	voices []ElevenLabsVoice
	ttl    time.Duration
}

var voicesCacheInst = &voicesCache{ttl: 5 * time.Minute}

// VoicesProxy hits ElevenLabs /v1/voices with the server's API key and caches
// the answer for 5 minutes. ElevenLabs throttles aggressively and the voice
// list barely changes, so we don't hammer it per request.
func (s *Server) VoicesProxy(w http.ResponseWriter, r *http.Request) {
	apiKey := os.Getenv("ELEVENLABS_API_KEY")
	if apiKey == "" {
		writeErr(w, http.StatusServiceUnavailable, "ELEVENLABS_API_KEY not set")
		return
	}
	voicesCacheInst.mu.Lock()
	cached := voicesCacheInst.voices
	fresh := time.Since(voicesCacheInst.at) < voicesCacheInst.ttl && len(cached) > 0
	voicesCacheInst.mu.Unlock()
	if fresh {
		writeJSON(w, http.StatusOK, cached)
		return
	}
	req, _ := http.NewRequestWithContext(r.Context(), http.MethodGet, "https://api.elevenlabs.io/v1/voices", nil)
	req.Header.Set("xi-api-key", apiKey)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		writeErr(w, resp.StatusCode, "elevenlabs: HTTP "+resp.Status)
		return
	}
	var payload struct {
		Voices []ElevenLabsVoice `json:"voices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		writeErr(w, http.StatusBadGateway, "elevenlabs: bad json")
		return
	}
	voicesCacheInst.mu.Lock()
	voicesCacheInst.voices = payload.Voices
	voicesCacheInst.at = time.Now()
	voicesCacheInst.mu.Unlock()
	writeJSON(w, http.StatusOK, payload.Voices)
}
