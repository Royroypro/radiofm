# RadioClub25 — tareas habituales. Ejecute `make help` para ver la lista.
# Go se busca en el PATH o en ~/.local/go/bin (instalación sin sudo).

GO ?= $(shell command -v go 2>/dev/null || echo $(HOME)/.local/go/bin/go)
STATICCHECK ?= $(shell command -v staticcheck 2>/dev/null || echo $(HOME)/go/bin/staticcheck)
# gofmt y staticcheck necesitan encontrar la misma instalación de Go
export PATH := $(dir $(GO)):$(PATH)
BIN := bin/radioweb
DEV_ENV := APP_ENV=development DATA_DIR=./data-dev WEB_DIR=./web HTTP_ADDR=127.0.0.1:8120 \
	SUPERADMIN_EMAIL=admin@local.test SUPERADMIN_PASSWORD=cambiar123

.PHONY: help dev build test lint fmt up down logs restart ps superadmin backup simulate

help: ## Muestra esta ayuda
	@grep -E '^[a-z-]+:.*## ' $(MAKEFILE_LIST) | awk 'BEGIN{FS=":.*## "}{printf "  \033[1m%-12s\033[0m %s\n",$$1,$$2}'

dev: ## Ejecuta la app en local (plantillas recargadas desde ./web, datos en ./data-dev)
	$(DEV_ENV) $(GO) run ./cmd/radioweb serve

build: ## Compila el binario en bin/radioweb
	CGO_ENABLED=0 $(GO) build -trimpath -o $(BIN) ./cmd/radioweb

test: ## Ejecuta todos los tests
	$(GO) test ./...

lint: ## gofmt + go vet + staticcheck (si está instalado)
	@test -z "$$(gofmt -l .)" || (echo "Archivos sin formatear:"; gofmt -l .; exit 1)
	$(GO) vet ./...
	@if [ -x "$(STATICCHECK)" ]; then $(STATICCHECK) ./...; else echo "staticcheck no instalado: $(GO) install honnef.co/go/tools/cmd/staticcheck@latest"; fi

fmt: ## Formatea el código
	gofmt -w .

up: ## Construye y levanta todos los servicios (docker compose)
	docker compose up -d --build

down: ## Detiene los servicios
	docker compose down

restart: ## Reinicia la app
	docker compose restart app

logs: ## Muestra los logs (Ctrl+C para salir)
	docker compose logs -f --tail=100

ps: ## Estado de los contenedores
	docker compose ps

superadmin: ## Crea o restablece un superadmin: make superadmin EMAIL=... PASS=...
	docker compose exec app radioweb create-superadmin "$(EMAIL)" "$(PASS)"

backup: ## Copia la base de datos y los archivos subidos a backups/
	@mkdir -p backups
	$(eval STAMP := $(shell date +%Y%m%d-%H%M%S))
	docker compose exec -T app radioweb backup /data/backup-$(STAMP).db
	docker compose exec -T app sh -c 'cd /data && tar czf - backup-$(STAMP).db uploads' > backups/radioclub25-$(STAMP).tar.gz
	docker compose exec -T app sh -c 'cd /data && mv backup-$(STAMP).db /tmp/'
	@ls -lh backups | tail -n 3

simulate: ## Simula la PC emisora con ffmpeg: make simulate TV_PASS=... [MODE=ambos] [SECS=60]
	SIM_HOST=localhost TV_PASS="$(TV_PASS)" test/simulate-broadcast.sh $(or $(MODE),ambos) $(or $(SECS),60)
