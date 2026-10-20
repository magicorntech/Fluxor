.PHONY: build test test-mongo lint server dev-server dashboard dashboard-install dashboard-build api-gen dev e2e

# --- Go ---
build:
	go build ./...

test:
	go test ./...
	cd dashboard && npm test && npm run build

# Store tests against a real MongoDB (brew services start mongodb/brew/mongodb-community)
MONGO_TEST_URI ?= mongodb://localhost:27017
test-mongo:
	FLUXOR_TEST_MONGO_URI=$(MONGO_TEST_URI) go test -count=1 ./internal/store/...

lint:
	golangci-lint run ./...

# --- Run (server and dashboard are separate processes; use two terminals) ---
# Uses MongoDB when FLUXOR_MONGO_URI is set, e.g. FLUXOR_MONGO_URI=mongodb://localhost:27017 make server
server:
	go run ./cmd/fluxor-server

# Server on a local MongoDB with a persistent dev key (see hack/dev-server.sh)
dev-server:
	./hack/dev-server.sh

dashboard:
	cd dashboard && npm run dev

# --- Dashboard ---
dashboard-install:
	cd dashboard && npm install

dashboard-build:
	cd dashboard && npm run build

# Regenerate the dashboard's TypeScript API types from api/openapi.yaml
api-gen:
	cd dashboard && npm run api:gen

# --- Later ---
dev:
	@echo "kind tabanlı dev ortamı ertelendi (docker şimdilik yok); 'make server' ve 'make dashboard' kullanın"

e2e:
	@echo "e2e testleri sonraki adımlarda eklenecek"
