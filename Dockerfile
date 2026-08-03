FROM node:26-alpine AS ui
WORKDIR /src/web
COPY web/package*.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

FROM golang:1.25-alpine AS backend
RUN apk add --no-cache build-base
WORKDIR /src
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
RUN CGO_ENABLED=1 go build -trimpath -ldflags="-s -w" -o /out/certvault .

FROM alpine:3.24
RUN apk add --no-cache ca-certificates tzdata && addgroup -S -g 10001 certvault && adduser -S -D -H -u 10001 -G certvault certvault
COPY --from=backend /out/certvault /usr/local/bin/certvault
COPY --from=ui /src/web/dist /app/ui
RUN mkdir /data && chown certvault:certvault /data
USER certvault
EXPOSE 8080
VOLUME ["/data"]
ENTRYPOINT ["/usr/local/bin/certvault"]
