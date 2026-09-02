# syntax=docker/dockerfile:1
FROM golang:1.25-bookworm AS build
ARG TARGETARCH
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 GOOS=linux GOARCH=$TARGETARCH go build -trimpath -ldflags='-s -w' -o /out/codelocal-pool ./cmd/codelocal-pool

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates && rm -rf /var/lib/apt/lists/*
RUN useradd --system --uid 10001 --create-home codelocal
WORKDIR /app
COPY --from=build /out/codelocal-pool /app/codelocal-pool
USER codelocal
ENV PORT=8080
EXPOSE 8080
ENTRYPOINT ["/app/codelocal-pool"]
