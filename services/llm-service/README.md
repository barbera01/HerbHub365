# llm-service

HTTP service that exposes `POST /generate` and proxies requests to the configured LLM provider.

## Gemini fallback

By default the service keeps using the local provider (`LLM_PROVIDER=auto` against `LLM_BASE_URL`) and will try Google Gemini only when the primary local AI endpoint is unavailable, for example connection refused, DNS failure, reset/EOF, or `502/503/504` responses.

Set one of these in the service environment:

```env
LLM_FALLBACK_PROVIDER=gemini
LLM_FALLBACK_API_KEY=your-google-ai-studio-key
# or: GEMINI_API_KEY=your-google-ai-studio-key
LLM_FALLBACK_MODEL=gemini-2.5-flash
```

Disable fallback with `LLM_FALLBACK_PROVIDER=none`.
