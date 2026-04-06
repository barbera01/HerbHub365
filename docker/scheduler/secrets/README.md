# Scheduler SSH secrets

This directory stores local files that Docker Compose mounts into the `schedular`
service as Docker secrets.

## Required files

- `id_ed25519` (private key, mode `600`)
- `known_hosts` (SSH host keys, mode `644`)

## Quick setup

```bash
mkdir -p scheduler/secrets
cp ~/.ssh/id_ed25519 scheduler/secrets/id_ed25519
chmod 600 scheduler/secrets/id_ed25519
ssh-keyscan <your-remote-host> > scheduler/secrets/known_hosts
chmod 644 scheduler/secrets/known_hosts
docker compose up -d schedular
```

## In Cronicle SSH plugin

- Private key path: `/run/secrets/scheduler_ssh_private_key`
- Known hosts path: `/run/secrets/scheduler_ssh_known_hosts`

If you keep these files elsewhere, set:

- `SCHEDULER_SSH_PRIVATE_KEY_FILE`
- `SCHEDULER_SSH_KNOWN_HOSTS_FILE`

in your environment before running `docker compose up`.
