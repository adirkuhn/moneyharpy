FROM node:24-alpine AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web ./
RUN npm run build

FROM golang:1.26-alpine AS server
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /src/web/dist ./web/dist
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /moneyharpy ./cmd/moneyharpy

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=server /moneyharpy /moneyharpy
EXPOSE 8080
ENTRYPOINT ["/moneyharpy"]
