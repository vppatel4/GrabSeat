// Small standalone binary used as the container healthcheck. It lives in the
// image so the healthcheck needs no shell, curl, or wget — it just asks the
// backend's own /healthz endpoint and exits non-zero if that fails.
package main

import (
	"net/http"
	"os"
	"time"
)

func main() {
	port := os.Getenv("BACKEND_PORT")
	if port == "" {
		port = "8080"
	}
	client := http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get("http://127.0.0.1:" + port + "/healthz")
	if err != nil {
		os.Exit(1)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		os.Exit(1)
	}
}
