# brainDB build targets. Run `make` or `make help` for the list.
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -X main.version=$(VERSION)
EXAMPLE ?= sales-company
BRAIN ?= ./data/brain.xml
PLATFORMS := linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64 windows/arm64

.DEFAULT_GOAL := help
.PHONY: install-skill help build go web test web-dev storybook release run mcp docker up down logs smoke smoke-docker clean

help: ## List targets
	@grep -E '^[a-zA-Z_-]+:.*## ' $(MAKEFILE_LIST) | awk 'BEGIN{FS=":.*## "}{printf "  %-14s %s\n", $$1, $$2}'

build: web go ## Build the web UI (if web/ exists) and bin/braindb

web: ## Build the web UI into internal/server/ui/dist
	@if [ -f web/package.json ]; then cd web && npm ci && npm run build; else echo "no web/, skipping UI build"; fi

go: ## Build only the Go binary bin/braindb
	go build -ldflags "$(LDFLAGS)" -o bin/braindb ./cmd/braindb

test: ## gofmt + go vet + go test + web typecheck and unit tests
	@unformatted="$$(gofmt -l .)"; if [ -n "$$unformatted" ]; then echo "not gofmt'd:"; echo "$$unformatted"; exit 1; fi
	go vet ./...
	go test ./...
	@if [ -f web/package.json ]; then cd web && npm run typecheck && npm test; fi

web-dev: ## Vite dev server with /api proxy to :7791
	cd web && npm run dev

storybook: ## Storybook on :6006
	cd web && npm run storybook

release: ## Cross-compile runtimes into dist/runtime/braindb-<os>-<arch>[.exe]
	@mkdir -p dist/runtime
	@for p in $(PLATFORMS); do os=$${p%/*}; arch=$${p#*/}; ext=; [ $$os = windows ] && ext=.exe; \
	  echo "building $$os/$$arch"; \
	  CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch go build -trimpath -ldflags "-s -w $(LDFLAGS)" -o dist/runtime/braindb-$$os-$$arch$$ext ./cmd/braindb || exit 1; \
	done

run: go ## Seed examples/$(EXAMPLE) into $(BRAIN) if missing, then serve it
	@mkdir -p $(dir $(BRAIN))
	@if [ ! -f $(BRAIN) ]; then \
	  if [ -d examples/$(EXAMPLE)/docs ]; then bin/braindb seed examples/$(EXAMPLE)/seed.json -o $(BRAIN) --ingest examples/$(EXAMPLE)/docs; \
	  else bin/braindb seed examples/$(EXAMPLE)/seed.json -o $(BRAIN); fi; \
	  if [ -d examples/$(EXAMPLE)/templates ] && [ ! -d $(dir $(BRAIN))templates ]; then cp -r examples/$(EXAMPLE)/templates $(dir $(BRAIN))templates; fi; \
	fi
	bin/braindb serve --brain $(BRAIN)

mcp: go ## Run the MCP stdio server on $(BRAIN)
	bin/braindb mcp --brain $(BRAIN)

docker: ## docker build -t braindb .
	docker build -t braindb .

up: ## docker compose up -d --build
	docker compose up -d --build

down: ## docker compose down
	docker compose down

logs: ## docker compose logs -f
	docker compose logs -f

smoke: ## HTTP smoke test against a running server
	sh scripts/smoke-test.sh

eval: go ## Retrieval-quality evaluation on the sales example (set BRAINDB_EMBED_* to score a remote engine)
	@rm -f /tmp/braindb-eval.xml
	./bin/braindb seed examples/sales-company/seed.json -o /tmp/braindb-eval.xml >/dev/null
	./bin/braindb eval --brain /tmp/braindb-eval.xml --gold examples/sales-company/eval.json

smoke-docker: ## Smoke test inside docker compose
	docker compose --profile test run --rm smoke

SKILL_DIR ?= ./.claude/skills
install-skill: ## Copy skills/braindb-bdql into $(SKILL_DIR) (default ./.claude/skills)
	@mkdir -p $(SKILL_DIR)
	cp -R skills/braindb-bdql $(SKILL_DIR)/
	@echo "installed braindb-bdql skill into $(SKILL_DIR)"

clean: ## Remove build outputs (bin/, dist/)
	rm -rf bin dist
