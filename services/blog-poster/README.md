# Blog Poster

`services/blog-poster` consumes `sensor.snapshots` messages from RabbitMQ, archives them by day, and uses a chat model such as your Ollama-hosted Qwen instance to write a Jekyll post into `hub/_posts`.

## Modes

- `daemon`: consume snapshots continuously and run an internal daily schedule to generate the post
- `collect`: only consume and archive queue messages
- `generate`: create one post from archived snapshots for `BLOG_TARGET_DATE`
- `draft`: pull one live queue message, keep it in RabbitMQ, and write a draft markdown file for testing
- `repo-post`: create an ad hoc technical post from selected repository files and a user prompt

This gives you two deployment styles:

- run one long-lived container with the built-in cron schedule
- run `collect` continuously and trigger `generate` from an external scheduler

## Archive flow

Each queue message is appended to `DATA_DIR/snapshots/YYYY-MM-DD.jsonl`.
The generator reads that archive, builds a summary, asks the model for markdown, and writes a Jekyll post file.

The existing GitHub Pages/Azure Static Web Apps workflow already deploys the `hub` site on push, so once the generated post is committed the site publish flow can pick it up.

## Optional PAT push

The service can optionally commit and push generated posts back to GitHub with a Personal Access Token. Draft runs do not push.

Required settings for PAT publishing:

- `GIT_PUBLISH_ENABLED=true`
- `GIT_PAT`: GitHub PAT with repo contents write access
- `GIT_REPO_DIR`: repo root mounted inside the container, defaults to `/repo`
- `GIT_REMOTE_NAME`: git remote name, defaults to `origin`
- `GIT_PUSH_BRANCH`: branch to push, defaults to `main`
- `GIT_AUTHOR_NAME` and `GIT_AUTHOR_EMAIL`: commit author identity

The publisher only stages and commits the generated post file, then pushes `HEAD` to the configured branch using an HTTPS GitHub remote plus an in-memory auth header derived from the PAT.

## Configuration

Copy the example file and adjust it for your environment:

```bash
cp services/blog-poster/.env.example services/blog-poster/.env
```

Important settings:

- `RABBITMQ_URL`: your AMQP connection string
- `RABBITMQ_QUEUE`: defaults to `sensor.snapshots`
- `LLM_PROVIDER`: `auto` (default), `ollama`, or `openai-compatible`
- `LLM_BASE_URL`: model endpoint base URL; the client now supports both OpenAI-compatible `/v1/chat/completions` and native Ollama `/api/chat`
- `LLM_API_KEY`: optional bearer token if your model gateway requires one
- `LLM_MODEL`: model name exposed by the gateway
- `LLM_TEMPERATURE`, `LLM_MAX_TOKENS`, and `LLM_REQUEST_TIMEOUT`: generation tuning controls
- `LLM_DEBUG`: log raw model responses for troubleshooting
- `HUB_DIR` and `BLOG_POSTS_DIR`: where the Jekyll site is mounted
- `BLOG_GENERATE_SCHEDULE`: cron expression for internal scheduling
- `BLOG_TARGET_DATE`: `today`, `yesterday`, or a `YYYY-MM-DD` date
- `BLOG_DRAFTS_DIR`: output directory for test drafts, defaults to `hub/_drafts`
- `BLOG_DRAFT_PREFIX`: filename prefix for draft runs
- `BLOG_REPO_POST_PROMPT`: ad hoc technical post request for `repo-post` mode
- `BLOG_REPO_POST_TITLE`: optional preferred title for `repo-post`
- `BLOG_REPO_POST_PATHS`: comma-separated repo paths to inspect; required for `repo-post`
- `BLOG_REPO_POST_DRAFT`: write repo posts to drafts by default for safety
- `BLOG_REPO_POST_CATEGORIES`: front matter categories for repo posts, defaults to `Platform Update`
- `GIT_PUBLISH_ENABLED`: enable commit and push after non-draft generation
- `GIT_PAT`: GitHub personal access token used for HTTPS push

`repo-post` has extra safety checks:

- it only reads the paths listed in `BLOG_REPO_POST_PATHS`
- it blocks common sensitive files such as `.env`, `*.pem`, `*.key`, `*.tfvars`, and token-like filenames
- it only includes a small set of text-based source files and trims total prompt size
- it redacts obvious secret/token patterns before sending context to the model

## Local development

From `services/blog-poster`:

```bash
go run ./cmd/blog-poster generate
```

Ad-hoc live draft test:

```bash
go run ./cmd/blog-poster draft
```

This reads a single message from `sensor.snapshots`, asks the model for a post, then writes a file like `hub/_drafts/draft-2026-03-15-some-title.markdown`. The RabbitMQ message is requeued so you can test repeatedly without consuming it.

If your model host is native Ollama rather than OpenAI-compatible, set `LLM_BASE_URL` to the Ollama host root such as `http://your-host:11434` or the routed base URL. By default the client tries `/v1/chat/completions` first and falls back to `/api/chat`. If your host is slow to load models or you want to skip the double-wait behavior, set `LLM_PROVIDER=ollama`.

Build locally:

```bash
go build ./...
```

Build the container:

```bash
docker build -t herbhub365/blog-poster -f dockerfile .
```

## Docker compose

The Docker service is defined in `docker/docker-compose.yml` and mounts:

- `..` to `/repo`
- `../services/blog-poster/data` to `/data`

If you want an external scheduler instead of the internal cron, override the command:

```bash
docker compose run --rm blog-poster generate
```

For a live draft test through Docker:

```bash
docker compose run --rm blog-poster draft
```

For an ad hoc repository explainer draft:

```bash
docker compose run --rm \
  -e BLOG_POSTER_MODE=repo-post \
  -e BLOG_REPO_POST_PROMPT="Create a blog post on the timelapse service explaining what it does and how it works." \
  -e BLOG_REPO_POST_TITLE="How the Herb Hub timelapse service works" \
  -e BLOG_REPO_POST_PATHS="services/timelapse-builder,scripts/make-timelapse.sh,docker/docker-compose.yml" \
  blog-poster
```

To publish an approved repository post instead of writing a draft, set:

```bash
BLOG_REPO_POST_DRAFT=false
GIT_PUBLISH_ENABLED=true
GIT_PAT=github_pat_your_token_here
```

To enable automatic push in Docker, provide a PAT in the environment before starting the container, for example:

```bash
export BLOG_POSTER_GIT_PUBLISH_ENABLED=true
export BLOG_POSTER_GIT_PAT=github_pat_your_token_here
docker compose up -d blog-poster
```

Useful Docker-side LLM overrides in `docker/.env`:

```bash
BLOG_POSTER_LLM_MODEL=qwen3.5:latest
BLOG_POSTER_LLM_MAX_TOKENS=1600
BLOG_POSTER_LLM_TEMPERATURE=0.6
BLOG_POSTER_LLM_REQUEST_TIMEOUT=5m
BLOG_POSTER_LLM_DEBUG=false
BLOG_POSTER_LLM_PROVIDER=ollama
```
