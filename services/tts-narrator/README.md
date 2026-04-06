# tts-narrator

Converts Jekyll blog posts into MP3 audio files using the self-hosted
[Kokoro FastAPI](https://github.com/remsky/Kokoro-FastAPI) TTS engine,
then patches the post's front matter with `audio_url` so the Jekyll site
can embed a native audio player.

## How it works

1. Reads a Jekyll post from `hub/_posts/`
2. Strips YAML front matter and Jekyll Liquid tags
3. Applies the TTS preprocessing pipeline (`config/tts-rules.json`) to
   convert markdown, units, symbols, dates, etc. into natural spoken-word text
4. POSTs the cleaned text to the Kokoro API and receives an MP3 back
5. Writes the MP3 to `hub/assets/audio/blog/YYYY-MM-DD-slug.mp3`
6. Injects `audio_url: /assets/audio/blog/YYYY-MM-DD-slug.mp3` into the
   post's YAML front matter
7. Optionally commits and pushes both files via `GIT_PAT`

The Jekyll `hub/_layouts/post.html` includes `audio-player.html` which renders
a native `<audio>` element whenever `page.audio_url` is set.

## Voices

| Name    | Kokoro voice string                              | Speed |
|---------|--------------------------------------------------|-------|
| `eve`   | `bf_lily(7)+bf_emma(2)+af_bella(1)+af_heart(1)` | 0.95  |
| `rowan` | `bm_daniel(7)+bm_lewis(3)`                       | 0.95  |

Set `TTS_VOICE=eve` (default), `TTS_VOICE=rowan`, or pass any raw Kokoro
voice string directly.

## Modes

| `TTS_MODE`  | Description                                                        |
|-------------|--------------------------------------------------------------------|
| `daemon`    | Cron-scheduled (default `10 0 * * *`), narrates posts each night  |
| `generate`  | Narrate the post(s) for `TTS_TARGET_DATE` and exit                |
| `backfill`  | Narrate every post that lacks `audio_url` and exit                |
| `dry-run`   | Preprocess + print text to stdout; no TTS call, no file writes    |

## Environment variables

| Variable                | Default                                                     | Description                                           |
|-------------------------|-------------------------------------------------------------|-------------------------------------------------------|
| `TTS_MODE`              | `daemon`                                                    | Run mode                                              |
| `TTS_SCHEDULE`          | `10 0 * * *`                                                | Cron schedule (daemon mode)                           |
| `TTS_RUN_ON_START`      | `false`                                                     | Run once immediately on daemon start                  |
| `TTS_TARGET_DATE`       | `yesterday`                                                 | `today`, `yesterday`, or `YYYY-MM-DD`                 |
| `TTS_TARGET_SLUG`       | _(empty)_                                                   | Slug fragment filter (generate mode)                  |
| `TTS_BASE_URL`          | `https://kokoro-api.lab.home-cloud.uk/v1/audio/speech`     | Kokoro API endpoint                                   |
| `TTS_MODEL`             | `kokoro`                                                    | Model name sent to the API                            |
| `TTS_VOICE`             | `eve`                                                       | Friendly voice name or raw Kokoro voice string        |
| `TTS_SPEED`             | `0` (use voice default)                                     | Override speed (0 = voice default)                    |
| `TTS_FORMAT`            | `mp3`                                                       | Audio format (`mp3`, `wav`, etc.)                     |
| `TTS_REQUEST_TIMEOUT`   | `120s`                                                      | HTTP timeout for the TTS call                         |
| `TTS_RULES_FILE`        | `/app/config/tts-rules.json`                                | Path to preprocessing rules JSON                      |
| `HUB_DIR`               | `/repo/hub`                                                 | Jekyll hub root                                       |
| `BLOG_POSTS_DIR`        | `$HUB_DIR/_posts`                                           | Posts directory                                       |
| `TTS_AUDIO_DIR`         | `$HUB_DIR/assets/audio/blog`                                | MP3 output directory                                  |
| `TTS_AUDIO_PUBLIC_PATH` | `/assets/audio/blog`                                        | URL path prefix for `audio_url` front matter value    |
| `GIT_PUBLISH_ENABLED`   | `false`                                                     | Commit and push after narration                       |
| `GIT_REPO_DIR`          | `/repo`                                                     | Repository root for git operations                    |
| `GIT_PAT`               | _(required if publish enabled)_                             | GitHub Personal Access Token                          |
| `GIT_AUTHOR_NAME`       | `Herb Hub Bot`                                              | Git commit author name                                |
| `GIT_AUTHOR_EMAIL`      | `bot@herbhub365.com`                                        | Git commit author email                               |

## Directory layout

```
services/tts-narrator/
├── cmd/tts-narrator/main.go          # entrypoint
├── config/tts-rules.json             # preprocessing rules (verbatim from lilly-test)
├── dockerfile
├── go.mod / go.sum
└── internal/
    ├── config/config.go              # env-based configuration
    ├── gitpublish/publisher.go       # git commit + push via PAT
    ├── post/
    │   ├── parser.go                 # find and read Jekyll posts
    │   └── patcher.go                # inject audio_url into front matter
    ├── preprocess/
    │   ├── rules.go                  # load + compile tts-rules.json
    │   └── preprocess.go             # strip Liquid/front matter, apply rules
    └── tts/
        ├── client.go                 # Kokoro HTTP client
        └── voices.go                 # friendly voice name resolution
```
