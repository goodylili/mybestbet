BINARIES := ingestor browserd resolver engine notifier api stats seed migrate
GO_DIRS := cmd internal pkg migrations
PG_ADMIN_URL ?= postgres://mybestbet:mybestbet@localhost:5433/postgres?sslmode=disable
TEST_DATABASE_URL ?= postgres://mybestbet:mybestbet@localhost:5433/mybestbet_test?sslmode=disable
TEST_REDIS_URL ?= redis://localhost:6379/15
LOADTEST_REDIS_URL ?= redis://localhost:6379/14
LOADTEST_ARGS ?=

.PHONY: ci fmt fmt-check vet lint test test-db build infra comments-check loadtest

ci: fmt-check comments-check build vet lint test

fmt:
	gofmt -s -w $(GO_DIRS)

fmt-check:
	@test -z "$$(gofmt -s -l $(GO_DIRS) | tee /dev/stderr)"

comments-check:
	@! grep -rnE '^\s*//' --include='*.go' $(GO_DIRS) | grep -vE '//(go:|nolint)'

vet:
	go vet ./...

lint:
	golangci-lint run ./...

test-db:
	@psql "$(PG_ADMIN_URL)" -tAc "SELECT 1 FROM pg_database WHERE datname = 'mybestbet_test'" | grep -q 1 || psql "$(PG_ADMIN_URL)" -c "CREATE DATABASE mybestbet_test"
	@DATABASE_URL=$(TEST_DATABASE_URL) go run ./cmd/migrate

test: test-db
	APP_ENV=test TEST_DATABASE_URL=$(TEST_DATABASE_URL) TEST_REDIS_URL=$(TEST_REDIS_URL) go test -race -count=1 ./...

build:
	@mkdir -p bin
	@for b in $(BINARIES); do go build -o bin/$$b ./cmd/$$b || exit 1; done

loadtest:
	go run ./cmd/loadtest -redis $(LOADTEST_REDIS_URL) $(LOADTEST_ARGS)

infra:
	docker compose -f ../deploy/docker-compose.yml up -d postgres redis

