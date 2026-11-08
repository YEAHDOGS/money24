FROM golang:1.24-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /newsd .

# Distroless: no shell, no package manager, CA certs included, runs as non-root.
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /newsd /newsd
EXPOSE 8024
ENTRYPOINT ["/newsd"]
