# Build from the repo root (single go.mod + internal/ in context):
#   docker build -f services/signaling/Dockerfile -t signaling .
FROM golang:1.26 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /bin/app ./services/signaling/cmd

FROM gcr.io/distroless/static-debian12
COPY --from=build /bin/app /app
ENTRYPOINT ["/app"]
