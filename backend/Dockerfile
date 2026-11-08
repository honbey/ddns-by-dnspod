# The backend stage uses BuildKit cache mounts: `docker build` and compose v2
# use BuildKit by default, so nothing has to be installed. No `# syntax`
# directive on purpose - it would pull another image from docker.io, and the
# built-in frontend already understands `RUN --mount`.

# Build the SPA first: it is embedded into the Go binary at compile time.
# npm defaults to the public registry here; override with
# --build-arg NPM_REGISTRY=https://registry.npmmirror.com on a slow network.
FROM node:24-slim AS frontend
ARG NPM_REGISTRY=https://registry.npmjs.org
WORKDIR /src/frontend
# Copy the lockfile first: npm ci installs exactly what package-lock.json pins,
# so this layer only rebuilds when the lockfile changes.
COPY frontend/package.json frontend/package-lock.json ./
# ci, not install: the lockfile is the input, so two builds of one commit
# produce one artifact.
RUN npm ci --no-audit --no-fund --registry=${NPM_REGISTRY}
COPY frontend/ ./
# prebuild regenerates src/api-types.gen.ts from ../backend/openapi.yml, so the
# contract has to sit at the same relative path it has in a checkout.
COPY backend/openapi.yml /src/backend/openapi.yml
# npm run build rather than npx vite build: the former runs tsc first, so type
# errors fail the build instead of shipping.
#
# BUILD_OUT_DIR aims the postbuild step at what Vite just wrote; its own default
# points at the local checkout layout, which does not exist inside a stage.
RUN BUILD_OUT_DIR=dist npm run build -- --outDir dist --emptyOutDir

# Keep this in step with the `go` directive in backend/go.mod.
# GOPROXY defaults to a mainland-China mirror; override with
# --build-arg GOPROXY=https://proxy.golang.org,direct elsewhere.
ARG GOPROXY=https://goproxy.cn,direct
FROM golang:1.27-trixie AS backend
ARG GOPROXY
ENV GOPROXY=${GOPROXY}
WORKDIR /src/backend
# The module cache is a build cache, not image content: mounting it keeps the
# downloads out of a layer while still making them visible to the steps below.
COPY backend/go.mod backend/go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY backend/ ./
# Trailing slashes: copy the contents of the dist directory, never the
# directory itself, so the embed finds dist/index.html.
COPY --from=frontend /src/frontend/dist/ ./internal/frontend/dist/
# The image is what ships, so the contract is checked here too: a handler that
# drifted from openapi.yml - a renamed field, an undocumented status - fails
# the build instead of the deploy. It only exercises what needs no DNSPod
# credential, so it stays quick and stays offline.
RUN --mount=type=cache,target=/go/pkg/mod \
	--mount=type=cache,target=/root/.cache/go-build \
	CGO_ENABLED=0 go test -count=1 -run 'TestContract' ./internal/api/
# No go mod tidy: it could change go.mod at build time and pull unexpected
# versions. Dependencies are pinned by go.sum.
# The version comes from openapi.yml, which is already in this stage: the
# binary can then say which release it is, and the answer cannot drift from the
# contract.
RUN --mount=type=cache,target=/go/pkg/mod \
	--mount=type=cache,target=/root/.cache/go-build \
	CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w -X ddns-by-dnspod/internal/config.Version=$(sed -n 's/^[[:space:]]*version: *\"\([^\"]*\)\".*/\1/p' openapi.yml)" \
    -o /out/ddns-server ./cmd/server
# The runtime image has no shell to create these with: the mount points exist
# in the image so the process can open its database and write certificates
# before a volume is mounted over either. /tmp is where SQLite spills a sort
# that does not fit in memory.
RUN mkdir -p /out/rootfs/data /out/rootfs/certs /out/rootfs/tmp \
	&& chmod 1777 /out/rootfs/tmp

# Scratch, on purpose: a static binary plus the CA bundle is the whole runtime.
# No distribution to patch, no shell to escape into, and the healthcheck below
# needs neither curl nor wget because the binary probes itself.
FROM scratch

# OIDC discovery, JWKS, the DNSPod API and the ACME directory are all HTTPS.
COPY --from=backend /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
# TZ is read from the zoneinfo database compiled into the binary (time/tzdata).
COPY --from=backend /out/rootfs/ /
COPY --from=backend /out/ddns-server /usr/local/bin/ddns-server

# /data holds the database and the key that decrypts the stored credentials,
# /certs holds the issued certificates: the four PEM files per certificate, with
# the names certbot uses. Both are volumes, and for /certs that is the point -
# another container reads the same files.
#
# The process runs as root. Under a rootless runtime (podman) that is the user
# running it: root inside the container is mapped to them outside, so the
# mounted directories are writable without a chown on the host, and a file
# written here is readable by their other containers. A deployment that would
# rather drop privileges can pass `--user <uid>:<gid>` and chown the two
# directories to match; nothing in the image depends on being root.
WORKDIR /

VOLUME ["/data", "/certs"]
EXPOSE 8080

ENV DDNS_DATABASE_PATH=/data/ddns.db \
    DDNS_CRYPTO_KEY_FILE=/data/secret.key \
    DDNS_ACME_OUTPUT_DIR=/certs \
    TZ=Asia/Shanghai

# Exec form, not a shell: this works in an image with no /bin/sh, which is what
# makes scratch or distroless an option. It asks /api/health (liveness);
# /api/ready also asks the identity provider, and a blinking IdP must not mark
# a working scheduler unhealthy.
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD ["/usr/local/bin/ddns-server", "-healthcheck"]

ENTRYPOINT ["/usr/local/bin/ddns-server"]
