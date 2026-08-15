# syntax=docker/dockerfile:1
FROM golang:1.25-bookworm AS build
ARG TARGETARCH
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
# Railway deploys only the Go cloud runtime. Keep the image build scoped to Go
# sources so docs/legacy TypeScript changes do not invalidate the compile layer.
# Full cross-platform tests already run in .github/workflows/ci.yml before merge.
COPY cmd ./cmd
COPY internal ./internal
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
