.PHONY: all build test race lint bench loadtest up down clean

all: test lint

build:
	go build -o bin/api ./cmd/api
	go build -o bin/consumer ./cmd/consumer

test:
	go test -v ./...

race:
	go test -race -v ./...

lint:
	golangci-lint run ./...

bench:
	go test -bench=. -benchmem ./...

loadtest:
	k6 run loadtest/ramp_up.js

up:
	docker compose -f deployments/docker-compose.yml up -d

down:
	docker compose -f deployments/docker-compose.yml down

clean:
	rm -rf bin/ coverage.html *.out
