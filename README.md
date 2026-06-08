# P2P Payment Ledger

A production-grade double-entry ledger service built in Go. Handles concurrent peer-to-peer transfers with serializable isolation and advisory locks, exposes a gRPC API, and ships with full observability and comprehensive test coverage.

**Stack:** Go 1.26 · PostgreSQL 16 · gRPC/protobuf v3 · pgx/v5 · sqlc · Docker Compose · Prometheus · Grafana

---

## Quick Start

**Requirements:** Docker, Docker Compose, grpcurl

```bash
git clone https://github.com/anmol420/p2p-payment-ledger
cd p2p-payment-ledger
docker compose up --build
```

Wait ~20 seconds for PostgreSQL to initialize and migrations to complete. You'll see:
```
ledger-server | {"level":"INFO","msg":"gRPC server listening","addr":":50051"}
```

### Create your first transfer

```bash
# Create Alice's account with ₹1000 (100,000 paise)
ALICE=$(grpcurl -plaintext \
  -d '{"owner_name":"Alice","initial_balance":100000}' \
  localhost:50051 ledger.v1.LedgerService.CreateAccount | \
  jq -r '.account.id')

# Create Bob's account (empty)
BOB=$(grpcurl -plaintext \
  -d '{"owner_name":"Bob","initial_balance":0}' \
  localhost:50051 ledger.v1.LedgerService.CreateAccount | \
  jq -r '.account.id')

# Transfer ₹200 (20,000 paise) from Alice to Bob
grpcurl -plaintext \
  -d "{\"from_account_id\":\"$ALICE\",\"to_account_id\":\"$BOB\",
       \"amount\":20000,\"idempotency_key\":\"txn-001\"}" \
  localhost:50051 ledger.v1.LedgerService.Transfer

# Check Alice's balance (should be 80,000)
grpcurl -plaintext \
  -d "{\"account_id\":\"$ALICE\"}" \
  localhost:50051 ledger.v1.LedgerService.GetBalance
```

---

## Architecture

```
┌─────────────────────────────────────┐
│            gRPC Clients             │
│     grpcurl • Go • TypeScript       │
└─────────────────┬───────────────────┘
                  │ HTTP/2 + Protobuf
                  ▼
┌─────────────────────────────────────┐
│         gRPC Server (:50051)        │
├─────────────────────────────────────┤
│ Interceptors                        │
│ • Recovery                          │
│ • Logging                           │
│ • Metrics                           │
│ • Tracing                           │
├─────────────────────────────────────┤
│ RPC Handlers                        │
│ • CreateAccount                     │
│ • GetBalance                        │
│ • Transfer                          │
│ • ListTransactions                  │
│ • GetAuditLog                       │
└─────────────────┬───────────────────┘
                  ▼
┌─────────────────────────────────────┐
│          Service Layer              │
├─────────────────────────────────────┤
│ TransferService                     │
│ • Serializable Transactions         │
│ • Advisory Locks                    │
│ • Idempotency                       │
│ • Balance Checks                    │
│                                     │
│ TransactionService                  │
│ • Cursor Pagination                 │
│ • Audit Queries                     │
└─────────────────┬───────────────────┘
                  ▼
┌─────────────────────────────────────┐
│     Repository (sqlc + pgx/v5)      │
├─────────────────────────────────────┤
│ • Type-safe Queries                 │
│ • Pooling                           │
│ • Prepared Statements               │
│ • Transaction Support               │
└─────────────────┬───────────────────┘
                  ▼
┌─────────────────────────────────────┐
│          PostgreSQL 18              │
├─────────────────────────────────────┤
│ accounts                            │
│ transactions                        │
│ audit_log                           │
│ schema_migrations                   │
└─────────────────────────────────────┘
```

---

## API Reference

All amounts are in **paise** (₹1 = 100 paise). Never use floats for money—always integers.

### CreateAccount

Create a new account with an optional initial balance.

```bash
grpcurl -plaintext \
  -d '{"owner_name":"Alice","initial_balance":100000}' \
  localhost:50051 ledger.v1.LedgerService.CreateAccount
```

**Response:**
```json
{
  "account": {
    "id": "550e8400-e29b-41d4-a716-446655440000",
    "owner_name": "Alice",
    "balance": "100000",
    "created_at": "2025-06-08T10:30:45Z",
    "updated_at": "2025-06-08T10:30:45Z"
  }
}
```

### GetBalance

Fetch the current balance and metadata for an account.

```bash
grpcurl -plaintext \
  -d '{"account_id":"550e8400-e29b-41d4-a716-446655440000"}' \
  localhost:50051 ledger.v1.LedgerService.GetBalance
```

### Transfer

Execute a transfer from one account to another. **Idempotent**—retrying with the same `idempotency_key` is safe.

```bash
grpcurl -plaintext \
  -d '{
    "from_account_id":"550e8400-e29b-41d4-a716-446655440000",
    "to_account_id":"660e8400-e29b-41d4-a716-446655440001",
    "amount":20000,
    "idempotency_key":"txn-001"
  }' \
  localhost:50051 ledger.v1.LedgerService.Transfer
```

**Possible errors:**
- `INVALID_ARGUMENT`: amount ≤ 0, missing fields
- `NOT_FOUND`: account does not exist
- `FAILED_PRECONDITION`: insufficient balance (from_account is debited but to_account would overdraft)
- `ALREADY_EXISTS`: same idempotency_key was processed—returns original response

### ListTransactions

Retrieve transactions for an account with cursor-based pagination.

```bash
# First page
grpcurl -plaintext \
  -d '{"account_id":"550e8400-e29b-41d4-a716-446655440000","page_size":20}' \
  localhost:50051 ledger.v1.LedgerService.ListTransactions

# Next page (use next_page_token from previous response)
grpcurl -plaintext \
  -d '{
    "account_id":"550e8400-e29b-41d4-a716-446655440000",
    "page_size":20,
    "page_token":"eyJjdXJzb3IiOiI5OTk5In0="
  }' \
  localhost:50051 ledger.v1.LedgerService.ListTransactions
```

**Response:**
```json
{
  "transactions": [
    {
      "id": "770e8400-e29b-41d4-a716-446655440002",
      "from_account_id": "550e8400-e29b-41d4-a716-446655440000",
      "to_account_id": "660e8400-e29b-41d4-a716-446655440001",
      "amount": "20000",
      "status": "COMPLETED",
      "created_at": "2025-06-08T10:35:12Z"
    }
  ],
  "next_page_token": "eyJjdXJzb3IiOiI5OTk4In0="
}
```

### GetAuditLog

View all balance changes for an account. Includes `balance_before` and `balance_after` for reconstruction at any point in time.

```bash
grpcurl -plaintext \
  -d '{"account_id":"550e8400-e29b-41d4-a716-446655440000","page_size":20}' \
  localhost:50051 ledger.v1.LedgerService.GetAuditLog
```

**Response:**
```json
{
  "entries": [
    {
      "id": "880e8400-e29b-41d4-a716-446655440003",
      "account_id": "550e8400-e29b-41d4-a716-446655440000",
      "transaction_id": "770e8400-e29b-41d4-a716-446655440002",
      "amount": "-20000",
      "balance_before": "100000",
      "balance_after": "80000",
      "created_at": "2025-06-08T10:35:12Z"
    }
  ],
  "next_page_token": ""
}
```

---

## Running Tests

```bash
# Unit tests (no Docker required—fast, ~2s)
go test -race -v ./internal/service/... ./internal/grpc/...

# Integration tests (requires Docker + real PostgreSQL)
docker compose up -d postgres migrate
go test -race -v -tags integration -timeout 120s ./...

# All tests with race detector and coverage
go test -race -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

**Test categories:**
- **Unit tests:** service logic, pagination, idempotency keys (no DB)
- **Integration tests:** gRPC handlers, DB queries, transactions

---

## Observability

### Metrics

Prometheus scrapes metrics from `:9090/metrics` every 15 seconds.

**Key metrics:**
- `ledger_transfer_duration_seconds{status="success|insufficient_funds|..."}` — histogram of transfer latency
- `ledger_transfer_total{status="..."}` — counter of transfer outcomes
- `ledger_transfer_amount_paise` — histogram of transfer amounts
- `grpc_server_handling_seconds` — RPC latency per method
- `grpc_server_started_total` — RPC call count per method

### Grafana

Dashboard auto-provisioned at **http://localhost:3000** (admin/admin):
- Transfer success rate and latency (p50, p95, p99)
- Account creation rate
- Database connection pool stats
- gRPC server health

### Structured Logs

```json
{
  "level": "INFO",
  "timestamp": "2025-06-08T10:35:12.123Z",
  "message": "transfer completed",
  "trace_id": "550e8400-e29b-41d4-a716-446655440000",
  "from_account_id": "550e8400-e29b-41d4-a716-446655440000",
  "to_account_id": "660e8400-e29b-41d4-a716-446655440001",
  "amount": "20000",
  "duration_ms": 145
}
```

Set `LOG_LEVEL` env var: `debug`, `info`, `warn`, `error`.

---

## Configuration

All configuration via environment variables. See `.env.example`:

```bash
# Database
DATABASE_URL=postgres://postgres:password@postgres:5432/ledger?sslmode=disable
DATABASE_MAX_CONNS=25
DATABASE_MIN_CONNS=5
DATABASE_TIMEOUT=5s

# Server
GRPC_PORT=50051
METRICS_PORT=9090
LOG_LEVEL=info

# Graceful shutdown
SHUTDOWN_TIMEOUT=30s
```

**Validation:**
- `DATABASE_URL` must be a valid postgres:// URI
- `*_TIMEOUT` must parse as Go duration (e.g., "5s", "30s")
- `LOG_LEVEL` must be one of: debug, info, warn, error
- Numeric ports must be 1–65535

If validation fails, the server panics on startup with a clear error message.

---

## Project Structure

```
p2p-payment-ledger/
├── cmd/
│   ├── server/
│   │   └── main.go              # gRPC server entrypoint, flag parsing
│   └── migrate/
│       ├── main.go              # standalone migration runner
│       └── migrations/           # *.up.sql, *.down.sql files
│
├── internal/
│   ├── config/
│   │   ├── config.go            # env var struct + validation
│   │   └── config_test.go
│   │
│   ├── db/
│   │   ├── db.go                # sqlc-generated DBTX interface
│   │   ├── models.go            # sqlc-generated data types
│   │   ├── querier.go           # sqlc-generated query interface
│   │   ├── queries.sql.go       # sqlc-generated SQL functions
│   │   ├── repository.go        # Repository interface + pgx adapter
│   │   ├── connect.go           # pgxpool.Pool factory
│   │   └── queries.sql          # SQL query definitions
│   │
│   ├── grpc/
│   │   ├── ledger_server.go     # gRPC handler implementations
│   │   ├── interceptors.go      # recovery, logging, metrics, tracing
│   │   └── interceptors_test.go
│   │
│   ├── service/
│   │   ├── transfer.go          # Transfer logic, advisory locks
│   │   ├── transaction.go       # Pagination, cursor encoding
│   │   ├── account.go           # Balance queries
│   │   └── *_test.go            # unit tests
│   │
│   ├── observability/
│   │   ├── logger.go            # slog setup with trace IDs
│   │   ├── metrics.go           # Prometheus collectors
│   │   └── trace.go             # correlation ID generation
│   │
│   └── shutdown/
│       └── graceful.go          # signal handling, drain
│
├── proto/
│   └── ledger/
│       └── v1/
│           ├── ledger.proto     # service definition
│           └── ledger.pb.go     # protoc-generated (checked in)
│
├── config/
│   ├── prometheus.yml           # Prometheus scrape config
│   └── grafana/
│       └── provisioning/         # dashboards + datasources
│
├── scripts/
│   └── smoke_test.sh            # quick e2e validation
│
├── docker-compose.yml           # postgres, server, prometheus, grafana
├── Dockerfile                   # multi-stage build
├── buf.yaml                     # protobuf lint + breaking changes
├── buf.gen.yaml                 # protobuf code generation config
├── go.mod                       # Go module definition
├── go.sum                       # dependency lock file
└── README.md                    # this file
```

---

## Development Workflow

### Setup

```bash
# Install dependencies
go mod download

# Generate protobuf code
buf generate

# Run tests
go test -race ./...

# Start services locally
docker compose up postgres prometheus grafana
go run ./cmd/server
```

### Making Schema Changes

1. Create a new migration:
   ```bash
   make migrate-create add_column
   make migrate-up
   ```

2. Update SQL queries in `internal/db/queries.sql`

3. Regenerate sqlc code:
   ```bash
   go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest
   sqlc generate
   ```

4. Update handlers/services in `internal/`

5. Run tests:
   ```bash
   go test -race ./...
   ```

### Making Proto Changes

1. Edit `proto/ledger/v1/ledger.proto`

2. Regenerate:
   ```bash
   buf generate
   ```

3. Update gRPC handlers in `internal/grpc/ledger_server.go`

4. Run tests

---

## Deployment

### Docker Images

Build multi-stage images:

```bash
docker compose build
```

**Images created:**
- `p2p-payment-ledger-server:latest` — gRPC server (distroless)
- `p2p-payment-ledger-migrate:latest` — migration runner (distroless)
---

## Troubleshooting

### "database connection timeout"

```bash
# Check PostgreSQL is healthy
docker compose logs postgres

# Verify DATABASE_URL is correct
echo $DATABASE_URL

# Check network connectivity
docker exec ledger-server nc -zv postgres 5432
```

### "advisory lock timeout"

Transfer took too long acquiring locks. This is rare but can happen under:
- Very high concurrency (> 500 concurrent transfers)
- Undersized `DATABASE_MAX_CONNS` (< 10)
- Database under heavy load

**Solution:** Increase pool size or reduce concurrency.

### "ALREADY_EXISTS: idempotency key"

Retried transfer with same `idempotency_key` within ~7 days. This is **expected behavior**—it means:
- Original transfer succeeded (or is being retried)
- Idempotency is working
- Response will be identical to first attempt

Use a unique key per transfer.

### Metrics not appearing in Prometheus

```bash
# Check Prometheus is scraping the server
curl http://localhost:9091/api/v1/targets

# Check metrics endpoint directly
curl http://localhost:9090/metrics | grep ledger_transfer
```

---

## Contributing

1. Fork and clone
2. Create a feature branch: `git checkout -b feat/your-feature`
3. Make changes and add tests
4. Run `go test -race ./...`
5. Commit with clear messages
6. Push and open a pull request
---
