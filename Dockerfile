FROM node:20-alpine AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json* ./
RUN npm ci
COPY web/ ./
RUN npm run build

FROM golang:1.24-bookworm AS backend
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY cmd/ ./cmd/
COPY internal/ ./internal/
RUN CGO_ENABLED=0 go build -o /out/dshare ./cmd/dshare

FROM gcr.io/distroless/static-debian12
WORKDIR /app
COPY --from=backend /out/dshare /app/dshare
COPY --from=web /src/web/dist /app/web/dist
ENV ADDR=:8080
ENV DATABASE_PATH=/data/dshare.db
ENV STATIC_DIR=/app/web/dist
VOLUME ["/data"]
EXPOSE 8080
ENTRYPOINT ["/app/dshare"]
