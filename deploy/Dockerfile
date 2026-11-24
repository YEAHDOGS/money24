# Money 24 images, built from the repo root: docker compose up -d --build (docker-compose.yml names the targets).
#   newsd : the news service
#   m24d  : the trading runner, with the desk pages built in (served at /desk/, the same build as the PC supervisor)
# Go is CGO-free, so the final images are distroless: no shell, CA certs included, non-root.

# The desk pages. Same build settings as scripts/desk-build.js: keep the two in step.
FROM node:22-alpine AS web
WORKDIR /src
COPY package.json package-lock.json ./
RUN npm ci --ignore-scripts
COPY . .
ENV M24_BASE=/desk/ VITE_TRADING_API=/desk VITE_NEWS_API=/desk/news VITE_SELF_HOSTED=1
RUN npx vite build --outDir /web --emptyOutDir

FROM golang:1.26-alpine AS build
ARG CMD
WORKDIR /src
COPY engine/go.mod engine/go.sum ./
RUN go mod download
COPY engine/ .
RUN CGO_ENABLED=0 go build -buildvcs=false -trimpath -ldflags="-s -w" -o /app ./cmd/${CMD} \
 && mkdir /data

FROM gcr.io/distroless/static-debian12:nonroot AS newsd
COPY --from=build /app /app
# Owned by the nonroot user so a fresh named volume mounted here is writable.
COPY --from=build --chown=nonroot:nonroot /data /data
ENV NEWSD_DATA=/data NEWSD_ADDR=0.0.0.0:8024
EXPOSE 8024
ENTRYPOINT ["/app"]

FROM gcr.io/distroless/static-debian12:nonroot AS m24d
COPY --from=build /app /app
COPY --from=build --chown=nonroot:nonroot /data /data
COPY --from=web /web /web
ENV M24D_WEB=/web M24D_BASE=/desk
EXPOSE 8025
ENTRYPOINT ["/app"]
