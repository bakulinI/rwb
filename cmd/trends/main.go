package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/nats-io/nats.go"

	"rwbtesttask/internal/trends"
)

func main() {
	cfg := config{
		httpAddr:             env("HTTP_ADDR", ":8080"),
		natsURL:              env("NATS_URL", nats.DefaultURL),
		natsSubject:          env("NATS_SUBJECT", "search.queries"),
		stopWords:            splitCSV(os.Getenv("STOP_WORDS")),
		maxPerSourceInWindow: envInt("MAX_PER_SOURCE_IN_WINDOW", 5),
	}

	store := trends.NewStore(5*time.Minute, cfg.maxPerSourceInWindow, cfg.stopWords)

	nc, err := nats.Connect(cfg.natsURL)
	if err != nil {
		log.Fatalf("connect to nats: %v", err)
	}
	defer nc.Close()

	sub, err := nc.Subscribe(cfg.natsSubject, func(msg *nats.Msg) {
		var event trends.SearchEvent
		if err := json.Unmarshal(msg.Data, &event); err != nil {
			log.Printf("bad search event: %v", err)
			return
		}
		store.Add(event, time.Now())
	})
	if err != nil {
		log.Fatalf("subscribe to nats: %v", err)
	}
	defer sub.Unsubscribe()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"service": "search trends",
			"endpoints": []string{
				"GET /health",
				"GET /top?limit=10",
				"POST /stopwords",
			},
		})
	})
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("GET /top", func(w http.ResponseWriter, r *http.Request) {
		limit := queryInt(r, "limit", 10)
		writeJSON(w, http.StatusOK, store.Top(limit))
	})
	mux.HandleFunc("POST /stopwords", func(w http.ResponseWriter, r *http.Request) {
		var words []string
		if err := json.NewDecoder(r.Body).Decode(&words); err != nil {
			writeError(w, http.StatusBadRequest, "expected JSON array of strings")
			return
		}
		store.SetStopWords(words)
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	server := &http.Server{
		Addr:              cfg.httpAddr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				store.PruneAndRebuild(time.Now())
			case <-ctx.Done():
				return
			}
		}
	}()

	go func() {
		log.Printf("http API listens on %s", cfg.httpAddr)
		log.Printf("nats subject: %s", cfg.natsSubject)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("http server: %v", err)
		}
	}()

	<-ctx.Done()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("http shutdown: %v", err)
	}
}

type config struct {
	httpAddr             string
	natsURL              string
	natsSubject          string
	stopWords            []string
	maxPerSourceInWindow int
}

func env(name string, fallback string) string {
	value := os.Getenv(name)
	if value == "" {
		return fallback
	}
	return value
}

func envInt(name string, fallback int) int {
	value := os.Getenv(name)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func splitCSV(value string) []string {
	if value == "" {
		return nil
	}

	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			result = append(result, part)
		}
	}

	return result
}

func queryInt(r *http.Request, name string, fallback int) int {
	value := r.URL.Query().Get(name)
	if value == "" {
		return fallback
	}

	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 1 {
		return fallback
	}

	return parsed
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		log.Printf("write json: %v", err)
	}
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
