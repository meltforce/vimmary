# Stage 1: Build frontend
FROM node:22.23.2-alpine AS frontend
WORKDIR /app/web
COPY web/package.json web/package-lock.json* ./
RUN npm ci
COPY web/ .
RUN npm run build

# Stage 2: Build Go binary
FROM golang:1.26-alpine AS backend
RUN apk add --no-cache git
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=frontend /app/web/dist ./web/dist

# CI passes VERSION=edge-<sha>. Without it linked into the binary, every build
# reports "dev" and nothing downstream can tell which commit is serving — which
# is what a deploy gate has to compare against.
ARG VERSION=dev
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
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
CMD ["./vimmary", "--config", "/data/config.yaml"]
