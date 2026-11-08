# syntax=docker/dockerfile:1

# Web UI, built once on the build machine's architecture.
FROM --platform=$BUILDPLATFORM node:26-alpine AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN mkdir -p ../internal/webui/dist && npm run build

# Hub binary, cross-compiled for the target platform.
FROM --platform=$BUILDPLATFORM golang:1.27-alpine AS build
ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev
ARG COMMIT=unknown
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /src/internal/webui/dist ./internal/webui/dist
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath \
    -ldflags "-s -w \
      -X github.com/xonoko/robust-home-automation/internal/version.Version=${VERSION} \
      -X github.com/xonoko/robust-home-automation/internal/version.Commit=${COMMIT}" \
    -o /out/hub ./cmd/hub

# Atlas CLI (community build, no login needed) to apply migrations in the init container.
FROM arigaio/atlas:1.3.8-community AS atlas

FROM gcr.io/distroless/static-debian13:nonroot
COPY --from=build /out/hub /usr/local/bin/hub
COPY --from=atlas /atlas /usr/local/bin/atlas
COPY internal/store/migrations /migrations
USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/hub"]
CMD ["serve"]
