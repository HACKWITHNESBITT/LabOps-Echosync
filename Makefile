.PHONY: up down infra build backend test web dev clean demo

up:          ## build + run everything via docker compose
	docker compose up --build

down:        ## stop everything
	docker compose down

infra:       ## only infrastructure (postgres + redis)
	docker compose up -d postgres redis

backend:     ## local backend build
	cd backend && go build ./...

test:        ## run all backend tests
	cd backend && go test -race -count=1 ./...

vet:         ## vet backend
	cd backend && go vet ./...

web:         ## install + build web command center
	cd web && npm install && npm run build

dev:         ## local backend run (requires infra up)
	cd backend && go run ./cmd/server

demo:        ## end-to-end demo: two simulated users, match + icebreaker + latency
	bash scripts/demo.sh

clean:
	docker compose down -v