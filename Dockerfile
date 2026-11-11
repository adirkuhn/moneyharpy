# The web and server stages run on the build machine and cross-compile, so
# images for other architectures build without emulation.
FROM --platform=$BUILDPLATFORM node:24-alpine AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web ./
RUN npm run build

FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS server
ARG TARGETOS TARGETARCH
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /src/web/dist ./web/dist
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w" -o /moneyharpy ./cmd/moneyharpy
# Uploads live here while no S3-compatible storage is configured. A volume
# mounted on it takes this ownership, so the non-root user can write.
RUN mkdir -p /out/data/uploads

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=server /moneyharpy /moneyharpy
COPY --from=server --chown=65532:65532 /out/data /data
EXPOSE 8080
ENTRYPOINT ["/moneyharpy"]
