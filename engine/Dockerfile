# One image per engine command. Build from engine/: docker build --build-arg CMD=newsd .
# CGO-free, so the final image is distroless: no shell, CA certs included, runs as non-root.
FROM golang:1.26-alpine AS build
ARG CMD=newsd
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -buildvcs=false -trimpath -ldflags="-s -w" -o /app ./cmd/${CMD} \
 && mkdir /data

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /app /app
# Owned by the nonroot user so a fresh named volume mounted here is writable.
COPY --from=build --chown=nonroot:nonroot /data /data
ENV NEWSD_DATA=/data NEWSD_ADDR=0.0.0.0:8024
EXPOSE 8024
ENTRYPOINT ["/app"]
