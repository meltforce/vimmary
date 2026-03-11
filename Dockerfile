# Stage 1: Build frontend
FROM node:22-alpine AS frontend
WORKDIR /app/web
COPY web/package.json web/package-lock.json* ./
RUN npm ci
COPY web/ .
RUN npm run build

# Stage 2: Build Go binary
FROM golang:1.25-alpine AS backend
RUN apk add --no-cache git
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=frontend /app/web/dist ./web/dist
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o vimmary ./cmd/vimmary

# Stage 3: Runtime
FROM alpine:3.21
ARG YTDLP_VERSION=2025.03.31
RUN apk add --no-cache ca-certificates ffmpeg \
    && wget -O /usr/local/bin/yt-dlp \
       "https://github.com/yt-dlp/yt-dlp/releases/download/${YTDLP_VERSION}/yt-dlp_linux" \
    && chmod +x /usr/local/bin/yt-dlp
WORKDIR /app
COPY --from=backend /app/vimmary .
COPY --from=backend /app/migrations /migrations
EXPOSE 443
CMD ["./vimmary", "--config", "/data/config.yaml"]
