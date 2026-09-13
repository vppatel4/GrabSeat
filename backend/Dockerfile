# Multi-stage build: compile static binaries, then ship them on a tiny
# distroless base with no shell and a non-root user.
FROM golang:1.23-alpine AS build
WORKDIR /src

# Cache module downloads separately from the source for faster rebuilds.
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -o /out/grabseat .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -o /out/healthcheck ./cmd/healthcheck

FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
COPY --from=build /out/grabseat /app/grabseat
COPY --from=build /out/healthcheck /app/healthcheck
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/app/grabseat"]
