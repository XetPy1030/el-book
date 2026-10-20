package emu

import (
	"embed"
	"encoding/json"
	"fmt"
	"image/png"
	"net/http"
	"strconv"

	"el-book/sdk/device"
)

//go:embed web/index.html
var webFS embed.FS

// Handler — HTTP-интерфейс эмулятора для браузера.
func (e *Emulator) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		data, _ := webFS.ReadFile("web/index.html")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(data)
	})
	mux.HandleFunc("GET /state", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, e.State())
	})
	mux.HandleFunc("GET /frame.png", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Cache-Control", "no-store")
		png.Encode(w, e.Panel())
	})
	mux.HandleFunc("GET /rect.png", func(w http.ResponseWriter, r *http.Request) {
		seq, _ := strconv.Atoi(r.URL.Query().Get("seq"))
		img, ok := e.RectPixels(seq)
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Cache-Control", "max-age=3600")
		enc := png.Encoder{CompressionLevel: png.BestSpeed}
		enc.Encode(w, img)
	})
	mux.HandleFunc("GET /events", e.serveEvents)
	mux.HandleFunc("POST /input", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Events []struct {
				T string `json:"t"`
				X int    `json:"x"`
				Y int    `json:"y"`
			} `json:"events"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		for _, ev := range req.Events {
			phase := map[string]device.TouchPhase{"down": device.TouchDown, "move": device.TouchMove, "up": device.TouchUp}[ev.T]
			e.Touch(phase, ev.X, ev.Y)
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("POST /key", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Key     string `json:"key"`
			Pressed bool   `json:"pressed"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		k, ok := keyNames[req.Key]
		if !ok {
			http.Error(w, "неизвестная кнопка "+req.Key, http.StatusBadRequest)
			return
		}
		e.Key(k, req.Pressed)
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("POST /usb", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Source    string `json:"source"`
			Connected bool   `json:"connected"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		src := device.SourceComputer
		if req.Source == "charger" {
			src = device.SourceCharger
		}
		e.PowerSource(src, req.Connected)
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("POST /battery", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Level    int  `json:"level"`
			Charging bool `json:"charging"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		e.SetBattery(clamp(req.Level, 0, 100), req.Charging)
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("POST /network", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Offline bool `json:"offline"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		e.SetOffline(req.Offline)
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("POST /orientation", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Rotation int `json:"rotation"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		e.Orientation(device.Rotation(clamp(req.Rotation, 0, 3)))
		w.WriteHeader(http.StatusNoContent)
	})
	return mux
}

var keyNames = map[string]device.Key{
	"power": device.KeyPower, "cover": device.KeySleepCover,
	"pgfwd": device.KeyPageForward, "pgback": device.KeyPageBack, "home": device.KeyHome,
}

// serveEvents — поток Server-Sent Events: обновления панели и смены состояния.
func (e *Emulator) serveEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "потоковая передача не поддерживается", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	ch := e.Subscribe()
	defer e.Unsubscribe(ch)
	send := func(kind string, v any) {
		data, _ := json.Marshal(v)
		fmt.Fprintf(w, "event: %s\ndata: %s\n\n", kind, data)
		flusher.Flush()
	}
	send("state", e.State())
	for {
		select {
		case <-r.Context().Done():
			return
		case rec := <-ch:
			if rec.Seq < 0 {
				send("state", e.State())
			} else {
				send("update", rec)
			}
		}
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}
