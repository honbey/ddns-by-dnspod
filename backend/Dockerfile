# BuildKit cache mounts work in `docker build` and compose v2. No `# syntax`
# directive: the built-in frontend understands `RUN --mount`.

FROM node:24-slim AS frontend
ARG NPM_REGISTRY=https://registry.npmjs.org
WORKDIR /src/frontend
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci --no-audit --no-fund --registry=${NPM_REGISTRY}
COPY frontend/ ./
# prebuild regenerates the SPA types from the contract at this relative path.
COPY backend/openapi.yml /src/backend/openapi.yml
# BUILD_OUT_DIR aims postbuild at the directory vite just wrote.
RUN BUILD_OUT_DIR=dist npm run build -- --outDir dist --emptyOutDir

# Keep in step with the `go` directive in backend/go.mod.
ARG GOPROXY=https://goproxy.cn,direct
FROM golang:1.27-trixie AS backend
ARG GOPROXY
ENV GOPROXY=${GOPROXY}
WORKDIR /src/backend
COPY backend/go.mod backend/go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY backend/ ./
# Trailing slash: copy the contents, so the embed finds dist/index.html.
COPY --from=frontend /src/frontend/dist/ ./internal/frontend/dist/
RUN --mount=type=cache,target=/go/pkg/mod \
	--mount=type=cache,target=/root/.cache/go-build \
	CGO_ENABLED=0 go test -count=1 -run 'TestContract' ./internal/api/
# Version comes from openapi.yml, the same source the SPA types use.
RUN --mount=type=cache,target=/go/pkg/mod \
	--mount=type=cache,target=/root/.cache/go-build \
	CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w -X ddns-by-dnspod/internal/config.Version=$(sed -n 's/^[[:space:]]*version: *\"\([^\"]*\)\".*/\1/p' openapi.yml)" \
    -o /out/ddns-server ./cmd/server
# /tmp is where SQLite spills a sort that does not fit in memory.
RUN mkdir -p /out/rootfs/data /out/rootfs/certs /out/rootfs/tmp \
	&& chmod 1777 /out/rootfs/tmp

FROM scratch
COPY --from=backend /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=backend /out/rootfs/ /
COPY --from=backend /out/ddns-server /usr/local/bin/ddns-server

WORKDIR /
VOLUME ["/data", "/certs"]
EXPOSE 8080

ENV DDNS_DATABASE_PATH=/data/ddns.db \
    DDNS_CRYPTO_KEY_FILE=/data/secret.key \
    DDNS_ACME_OUTPUT_DIR=/certs \
    TZ=Asia/Shanghai

# Exec form needs no shell. It asks /api/health: /api/ready also checks the IdP,
# which must not make a working scheduler unhealthy.
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD ["/usr/local/bin/ddns-server", "-healthcheck"]

ENTRYPOINT ["/usr/local/bin/ddns-server"]
