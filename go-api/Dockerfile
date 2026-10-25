# Production image: MySQL + bcrypt + RabbitMQ adapters are compiled in via `-tags prod`.
# `go mod tidy` resolves go.sum on first build (needs network); commit the generated go.sum after.
FROM golang:1.24 AS build
WORKDIR /src
COPY . .
RUN go mod tidy && CGO_ENABLED=0 go build -tags prod -trimpath -ldflags="-s -w" -o /out/api ./cmd/api

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/api /api
EXPOSE 8080
USER nonroot
ENTRYPOINT ["/api"]
