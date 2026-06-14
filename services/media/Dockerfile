# Build from the repo root (single go.mod + internal/ in context):
#   docker build -f services/media/Dockerfile -t media .
FROM golang:1.26 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /bin/app ./services/media/cmd

FROM debian:12-slim
RUN apt-get update \
    && apt-get install -y --no-install-recommends ffmpeg ca-certificates \
    && rm -rf /var/lib/apt/lists/*
COPY --from=build /bin/app /app
ENTRYPOINT ["/app"]
