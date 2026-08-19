# syntax=docker/dockerfile:1
FROM golang:1.25-alpine AS deps
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

FROM deps AS build
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/bidcraft ./cmd/api
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/stress   ./cmd/stress

# The race detector needs cgo, which needs a C toolchain -- absent from the
# `runtime` stage's distroless base by design. This stage exists so
# `docker compose run --rm backend-test` can run `go test -race` reliably
# from any host, including Windows (where enabling cgo locally requires a
# separate MinGW install `go test -race` cannot do without).
FROM deps AS test
RUN apk add --no-cache gcc musl-dev
COPY . .
ENV CGO_ENABLED=1
CMD ["go", "test", "-race", "-count=1", "./..."]

FROM gcr.io/distroless/static-debian12:nonroot AS runtime
COPY --from=build /out/bidcraft /app/bidcraft
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/app/bidcraft"]
