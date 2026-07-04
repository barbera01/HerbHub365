# llm-service

HTTP service that exposes `POST /generate` and proxies requests to the configured LLM provider.

## Endpoints

- `POST /generate` — generate markdown from `system_prompt` + `user_prompt`, optionally with an image.
- `POST /warm` — keep a local Ollama model loaded (no-op for OpenAI/Gemini).
- `GET /health` — liveness probe.

## Request body

### Text only

```json
{
  "system_prompt": "You are a greenhouse blogger...",
  "user_prompt": "Write a daily update..."
}
```

### With an image

```json
{
  "system_prompt": "You are a greenhouse blogger...",
  "user_prompt": "Describe what you see in the attached image...",
  "image_data": "/9j/4AAQ...",
  "image_mime_type": "image/jpeg"
}
```

`image_data` can be a raw base64 string or a data URI such as `data:image/jpeg;base64,/9j/4AAQ...`. When a data URI is supplied, `image_mime_type` is optional.

Supported image formats depend on the provider; JPEG, PNG, and WebP are widely supported.

## Configuration

| Environment variable | Default | Description |
|---|---|---|
| `LLM_SERVICE_LISTEN_ADDR` | `:8080` | HTTP listen address |
| `LLM_SERVICE_SHUTDOWN_TIMEOUT` | `25m` | Graceful shutdown timeout |
| `LLM_SERVICE_MAX_CONCURRENT` | `1` | Maximum concurrent `/generate` requests; extra requests are rejected with `503` |
| `LLM_PROVIDER` | `auto` | `auto`, `ollama`, `openai`, `openai-compatible`, or `gemini` |
| `LLM_BASE_URL` | `http://ollama.la.home-cloud.uk` | Provider base URL |
| `LLM_API_KEY` | — | Optional bearer token for OpenAI-compatible endpoints |
| `LLM_MODEL` | `qwen2.5:latest` | Model name |
| `LLM_TEMPERATURE` | `0.6` | Sampling temperature |
| `LLM_TOP_P` | `0.9` | Nucleus sampling |
| `LLM_REPEAT_PENALTY` | `1.1` | Repeat penalty (Ollama) |
| `LLM_MAX_TOKENS` | `900` | Maximum tokens to generate |
| `LLM_REQUEST_TIMEOUT` | `20m` | Per-request hard timeout |
| `LLM_RESPONSE_HEADER_TIMEOUT` | `60s` | How long to wait for the first response byte |
| `LLM_DEBUG` | `false` | Log raw provider responses |
| `LLM_FALLBACK_PROVIDER` | `gemini` | Fallback provider; set to `none` to disable |
| `LLM_FALLBACK_BASE_URL` | `https://generativelanguage.googleapis.com/v1beta` | Fallback base URL |
| `LLM_FALLBACK_API_KEY` | value of `GEMINI_API_KEY` | Fallback API key |
| `LLM_FALLBACK_MODEL` | `gemini-3.1-flash-lite-preview` | Fallback model |

## Provider-specific image notes

- **Gemini** (`gemini-*`) — natively supports image `inlineData` parts. This is the recommended fallback when images are supplied.
- **OpenAI-compatible** — sends a `content` array with `text` and `image_url` parts. The image is embedded as a base64 `data:` URI.
- **Ollama** — passes base64 images in the `images` array of the user message. Only vision-capable models such as `llava`, `bakllava`, `moondream`, or `qwen2.5-vl` will work.

## Gemini fallback

By default the service keeps using the local provider (`LLM_PROVIDER=auto` against `LLM_BASE_URL`) and will try Google Gemini when the primary endpoint is unavailable, for example connection refused, DNS failure, reset/EOF, or `502/503/504` responses.

When an image is supplied, the fallback provider is forced to Gemini unless it is already one of the vision-capable providers (`gemini`, `openai`, `openai-compatible`, `ollama`). This ensures image requests do not fall back to a text-only model.

Set one of these in the service environment:

```env
LLM_FALLBACK_PROVIDER=gemini
LLM_FALLBACK_API_KEY=your-google-ai-studio-key
# or: GEMINI_API_KEY=your-google-ai-studio-key
LLM_FALLBACK_MODEL=gemini-2.5-flash
```

Disable fallback with `LLM_FALLBACK_PROVIDER=none`.

## Request size limits

`POST /generate` accepts request bodies up to 32 MiB. This should comfortably fit a base64-encoded JPEG/PNG plus JSON overhead. Tune the limit via `LLM_SERVICE_MAX_BODY_BYTES` if you expect very large images.
