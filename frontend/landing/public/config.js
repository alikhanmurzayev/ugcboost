// Local-dev fallback. Production / staging overwrite this file via the
// nginx entrypoint with the same shape (window.__RUNTIME_CONFIG__).
// Port 8080 matches the local `go run` backend (Astro dev / Playwright
// webServer); docker compose backend listens on 8082 and is consumed by
// `make start-landing` via the entrypoint script, not this file.
window.__RUNTIME_CONFIG__ = { apiUrl: "http://localhost:8080" };
