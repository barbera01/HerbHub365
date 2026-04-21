# Migrating video-narrator to a GPU Machine

This guide covers moving the `video-narrator` service from the main HerbHub server
to a dedicated GPU machine. A NAS provides the NFS share that both machines mount,
giving `video-publisher` on the main server instant access to MP4s written by
`video-narrator` on the GPU machine.

## Architecture after migration

```
                        NAS
                   /volume/herbhub-video
                   (NFS share)
                  ↗               ↖
        Main server              GPU machine
        ───────────────          ─────────────────────────
        herbhub-video            video-narrator (:8090)
        video-publisher               │  submits jobs to MuseTalk
        rabbitmq                      │  ffmpeg stitches on completion
                                      │  writes MP4 → NFS mount
             ↑                        │
             └──── AMQP notification ─┘
                   (video.produced)
```

`herbhub-video` resolves post content locally and sends it inline in the generate
request, so `video-narrator` never needs access to the repo or posts directory.

---

## Prerequisites

- NAS with NFS server enabled and a shared volume (e.g. `/volume1/herbhub-video`)
- GPU machine running Docker (and Docker Compose v2)
- Both the main server and GPU machine can reach the NAS over NFS
- Firewall rules permitting:
  - TCP 8090 — main server → GPU machine (video-narrator HTTP)
  - TCP 5672 — GPU machine → main server (RabbitMQ AMQP)
  - NFS ports (TCP/UDP 2049) — both machines → NAS

---

## Step 1 — Configure the NFS share on the NAS

These steps are for a Synology NAS. Adjust for your brand.

1. **File Services → NFS** — enable NFS service.
2. **Shared Folder → your folder → Edit → NFS Permissions → Create**:
   - Add a rule for the **main server** IP:
     - Privilege: Read/Write
     - Squash: No mapping (or Map root to admin)
     - Enable asynchronous: yes
   - Add a rule for the **GPU machine** IP with the same settings.
3. Note the NAS export path shown in the NFS Permissions dialog, e.g.
   `/volume1/herbhub-video`.

---

## Step 2 — Mount the NFS share on the main server

```bash
sudo apt install nfs-common

sudo mkdir -p /mnt/herbhub-video

# Test mount (replace NAS_IP and export path as needed)
sudo mount NAS_IP:/volume1/herbhub-video /mnt/herbhub-video

# Verify
ls /mnt/herbhub-video
```

Make the mount persistent — add to `/etc/fstab`:

```
NAS_IP:/volume1/herbhub-video  /mnt/herbhub-video  nfs  defaults,_netdev  0  0
```

---

## Step 3 — Mount the NFS share on the GPU machine

```bash
sudo apt install nfs-common

sudo mkdir -p /mnt/herbhub-video

sudo mount NAS_IP:/volume1/herbhub-video /mnt/herbhub-video

ls /mnt/herbhub-video
```

Add to `/etc/fstab` on the GPU machine too:

```
NAS_IP:/volume1/herbhub-video  /mnt/herbhub-video  nfs  defaults,_netdev  0  0
```

---

## Step 4 — Update the main server's Docker volumes

The `video-publisher` and `herbhub-video` services must now bind the NFS mount
instead of the previous local Docker volume.

Edit `docker/docker-compose.yml` on the **main server**:

```yaml
herbhub-video:
  volumes:
    - ..:/repo
    - /mnt/herbhub-video:/output/video   # was: local Docker volume

video-publisher:
  volumes:
    - /mnt/herbhub-video:/output/video   # was: local Docker volume
    - /etc/youtube:/secrets/youtube
    - ..:/repo
```

---

## Step 5 — Update NARRATOR_URL on the main server

In `docker/docker-compose.yml`, change the `herbhub-video` environment:

```yaml
herbhub-video:
  environment:
    NARRATOR_URL: https://video-narrator.lab.home-cloud.uk   # was: http://video-narrator:8080
```

---

## Step 6 — Place intro/outro resources on the NAS

Both machines access resources from the NFS mount, so no copying between hosts
is needed. Place the files on the NAS under the same share:

```
/volume1/herbhub-video/
  Resources/
    video/               ← intro.mp4, outro.mp4, etc.
    video_backgrounds/   ← background images for chroma-key
```

The GPU machine will mount these at `/mnt/herbhub-video/Resources/video` and
`/mnt/herbhub-video/Resources/video_backgrounds` automatically via the compose file.

---

## Step 7 — Deploy video-narrator on the GPU machine

Clone the repo onto the GPU machine:

```bash
git clone https://github.com/YOUR_ORG/HerbHub365.git /opt/herbhub/repo
```

Create `/opt/herbhub/repo/docker/.env`:

```env
VIDEO_BASE_URL=http://localhost:8011
RABBITMQ_URL=amqp://admin:yourpassword@MAIN_SERVER_IP:5672/
NFS_OUTPUT_MOUNT=/mnt/herbhub-video/Video/Output
# Resources are read directly from the NAS NFS mount — no local copies needed
VIDEO_RESOURCES_PATH=/mnt/herbhub-video/Resources/video
VIDEO_BG_RESOURCES_PATH=/mnt/herbhub-video/Resources/video_backgrounds
```

Build and start:

```bash
cd /opt/herbhub/repo/docker
docker compose -f docker-compose.gpu-narrator.yml up -d --build
```

Confirm healthy (both local and via Traefik):

```bash
# Direct (on the GPU machine)
curl http://localhost:8090/api/health

# Via Traefik
curl https://video-narrator.lab.home-cloud.uk/api/health
# {"status":"ok"}
```

---

## Step 8 — Remove video-narrator from the main server

Once the GPU machine is confirmed healthy:

```bash
# On the main server
cd /path/to/HerbHub365/docker
docker compose stop video-narrator
docker compose rm -f video-narrator
```

Restart the main stack to pick up the volume and `NARRATOR_URL` changes:

```bash
docker compose up -d herbhub-video video-publisher
```

---

## Step 9 — End-to-end test

1. Trigger a video generation:
   ```bash
   curl -X POST https://video-narrator.lab.home-cloud.uk/api/generate \
     -H 'Content-Type: application/json' \
     -d '{"slug":"YOUR_POST_SLUG"}'
   ```
2. Watch GPU machine logs:
   ```bash
   docker logs -f video-narrator
   ```
3. After "stitching" completes, confirm the MP4 appears on the NAS mount on
   both machines:
   ```bash
   # GPU machine
   ls -lh /mnt/herbhub-video/
   # Main server
   ls -lh /mnt/herbhub-video/
   ```
4. Confirm `video-publisher` picks it up from RabbitMQ and uploads to YouTube.

---

## Rollback

1. Set `NARRATOR_URL` back to `http://video-narrator:8080` in `docker-compose.yml`.
2. Restore the `video-narrator` service block (or uncomment it).
3. Revert volume mounts back to the local Docker volume.
4. Run `docker compose up -d` on the main server.

---

## Environment variable reference (GPU machine)

| Variable | Description | Default |
|---|---|---|
| `VIDEO_BASE_URL` | MuseTalk API URL on the GPU machine | `http://localhost:8011` |
| `RABBITMQ_URL` | AMQP URL on the main server | _(required)_ |
| `NFS_OUTPUT_MOUNT` | Local path of the shared `Video/Output` directory | `/mnt/herbhub-video/Video/Output` |
| `VIDEO_RESOURCES_PATH` | Intro/outro MP4s (on NAS) | `/mnt/herbhub-video/Resources/video` |
| `VIDEO_BG_RESOURCES_PATH` | Background images (on NAS) | `/mnt/herbhub-video/Resources/video_backgrounds` |
| `VIDEO_AVATAR` | Default avatar ID | `rowan` |
| `CONCAT_VIDEO_CODEC` | ffmpeg encoder (`h264_nvenc` for GPU, `libx264` for CPU) | `h264_nvenc` |
| `CONCAT_CRF` | Quality factor (`-cq` for NVENC, `-crf` for libx264) | `18` |
| `CONCAT_PRESET` | Encoder preset (`p1`–`p7` for NVENC, `fast`/`slow` etc for libx264) | `p4` |
| `CONCAT_THREADS` | CPU thread limit (software encode only) | `1` |
| `CHROMA_KEY_ENABLED` | Enable green-screen removal | `false` |
