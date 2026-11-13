# Stage 1: Build frontend
FROM node:26.13.1-alpine AS frontend
WORKDIR /app/web
COPY web/package.json web/package-lock.json* ./
# The cache mounts in this file survive between builds on one Docker host, so
# a local rebuild re-resolves rather than re-downloads; a CI runner starts
# without them.
RUN --mount=type=cache,target=/root/.npm npm ci
COPY web/ .
RUN npm run build

# Stage 2: Build Go binary
FROM golang:1.27-alpine AS backend
RUN apk add --no-cache git
WORKDIR /app
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY . .
COPY --from=frontend /app/web/dist ./web/dist

# deploy.yml and preview.yml pass the full commit SHA, release.yml the tag.
# /version and the loopback /healthz report it as "build"; pipeline-deploy
# waits for the SHA on /version. Without it every build reports "dev".
ARG VERSION=dev
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-s -w -X main.Version=${VERSION}" -o vimmary ./cmd/vimmary

# Stage 3: Runtime
FROM alpine:3.24
# ghcr links a package to its repo through this label — without it the package
# shows up unattached and inherits no visibility or README from the repo.
LABEL org.opencontainers.image.source="https://github.com/meltforce/vimmary"
RUN apk add --no-cache ca-certificates ffmpeg python3 py3-pip \
    && pip3 install --break-system-packages yt-dlp
WORKDIR /app
COPY --from=backend /app/vimmary .
COPY --from=backend /app/migrations /migrations
EXPOSE 443

# The probe lives in the image, not in a compose file, because a compose
# healthcheck is owned by whichever repo holds the deployment. The one added to
# homelab on 2026-08-06 lasted 94 seconds: it correctly reported a start that
# never opened its listener, the auto-rollback read that unhealthy container as
# a fault of the commit that introduced the check, and reverted it. vimmary then
# ran dead for 6h23min with nothing watching. Baked in here, no deployment-side
# revert removes it. See INCIDENTS.md, 2026-08-07.
#
# 127.0.0.1:8081 is the default of health_addr, and StartHealthListener runs as
# the last step of startup — the endpoint answering is itself the readiness
# signal. A deployment that moves health_addr must override this HEALTHCHECK
# along with it. The listener is on loopback because with Tailscale enabled the
# real listener is on the tsnet netstack, which nothing inside the container can
# dial.
#
# start-period covers the startup worst case, which is now 90 s for tsnet Up
# plus migrations — the 30 s setec budget it used to include is gone with the
# setec client. A healthy start reaches the listener in well under a second
# after tsnet; the margin is for migrations on a cold database.
HEALTHCHECK --interval=30s --timeout=5s --start-period=120s --retries=3 \
    CMD wget -q -O /dev/null http://127.0.0.1:8081/healthz || exit 1

CMD ["./vimmary", "--config", "/data/config.yaml"]
