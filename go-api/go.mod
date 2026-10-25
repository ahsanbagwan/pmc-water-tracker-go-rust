module pmc

go 1.24

// Production-only dependencies (used behind the `prod` build tag, see internal/*/ *_prod.go).
// Run `go mod tidy` once with network access to produce go.sum:
//   go mod tidy && go build -tags prod ./cmd/api
require (
	github.com/go-sql-driver/mysql v1.8.1
	github.com/rabbitmq/amqp091-go v1.10.0
	golang.org/x/crypto v0.31.0
)
