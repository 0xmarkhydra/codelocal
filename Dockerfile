# syntax=docker/dockerfile:1
FROM golang:1.25-bookworm AS build
ARG TARGETARCH
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# Compile/test the full Go tree so npm/client regressions cannot ride along with
# an otherwise healthy cloud-only build.
RUN go test ./...
RUN CGO_ENABLED=0 GOOS=linux GOARCH=$TARGETARCH go build -trimpath -ldflags='-s -w' -o /out/codelocal-cloud ./cmd/codelocal-cloud

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates && rm -rf /var/lib/apt/lists/*
RUN useradd --system --uid 10001 --create-home codelocal
WORKDIR /app
COPY --from=build /out/codelocal-cloud /app/codelocal-cloud
COPY assets /app/assets
USER codelocal
ENV PORT=3333
EXPOSE 3333
ENTRYPOINT ["/app/codelocal-cloud"]
