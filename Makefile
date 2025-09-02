# ---- Makefile ----------------------------------------------------------------
# Loads .env if present, exports vars so docker compose and mysql (inside container)
# use the same values.

ifneq (,$(wildcard .env))
	include .env
	export
endif

# --- Repo / Paths -------------------------------------------------------------

# Absolute path to directory that contains this Makefile (repo root)
ROOT_DIR := $(abspath $(dir $(lastword $(MAKEFILE_LIST))))

# Defaults (override in .env)
PROJECT_NAME        ?= cupid_hotel
COMPOSE_FILE        ?= docker/compose.yml
MYSQL_SERVICE       ?= mysql

# MySQL defaults (override in .env)
MYSQL_HOST          ?= 127.0.0.1
MYSQL_PORT          ?= 3307
MYSQL_USER          ?= root
MYSQL_ROOT_PASSWORD ?= root
MYSQL_PASSWORD      ?= root
MYSQL_DATABASE      ?= cupid

# Use the root or non-root password depending on MYSQL_USER
ifeq ($(MYSQL_USER),root)
  MYSQL_PASS := $(MYSQL_ROOT_PASSWORD)
else
  MYSQL_PASS := $(MYSQL_PASSWORD)
endif

# Migrations (repo-relative)
MIGRATIONS_DIR_REL  ?= internal/storage/mysql/migrations
# Absolute path used for tests and scripts
MIGRATIONS_DIR_ABS  := $(ROOT_DIR)/$(MIGRATIONS_DIR_REL)
# Some test helpers also look here; we'll ensure a mirror exists.
MIGRATIONS_MIRROR_REL ?= storage/mysql/migrations
MIGRATIONS_MIRROR_ABS := $(ROOT_DIR)/$(MIGRATIONS_MIRROR_REL)

# Always point docker compose at ABSOLUTE files so IDE CWD doesn't matter
COMPOSE := docker compose --env-file "$(ROOT_DIR)/.env" -f "$(ROOT_DIR)/$(COMPOSE_FILE)"

SHELL := /bin/bash

.PHONY: up down stop restart ps logs build mysql sh migrate remigrate \
	verify ping test itest lint fmt help nuke rebuild wait-mysql reset ingest \
	ensure-migrations

help:
	@echo ""
	@echo "Targets:"
	@echo "  up          - Start services (detached)"
	@echo "  down        - Stop and remove services"
	@echo "  stop        - Stop services"
	@echo "  restart     - Restart services"
	@echo "  ps          - Show service status"
	@echo "  logs        - Tail MySQL logs"
	@echo "  build       - Rebuild images"
	@echo "  mysql       - Open mysql client inside the container (as $(MYSQL_USER))"
	@echo "  sh          - Shell into the mysql container"
	@echo "  migrate     - Apply migrations (skip 1_init.sql; entrypoint already runs it)"
	@echo "  remigrate   - Drop & recreate DB (root only) then migrate"
	@echo "  verify      - Show tables and describe a key table"
	@echo "  ping        - Print server version (via container, as $(MYSQL_USER))"
	@echo "  fmt/lint    - Format and lint Go code"
	@echo "  test        - Run unit tests (no cache)"
	@echo "  itest       - Run integration tests (no cache, integration tag, MIGRATIONS_DIR exported)"
	@echo ""

# --- Docker lifecycle ---------------------------------------------------------

up:
	@$(COMPOSE) up -d

down:
	@$(COMPOSE) down

stop:
	@$(COMPOSE) stop

restart: stop up

ps:
	@$(COMPOSE) ps

logs:
	@$(COMPOSE) logs -f $(MYSQL_SERVICE)

build:
	@$(COMPOSE) build --no-cache

# --- MySQL convenience --------------------------------------------------------

# Interactive mysql *inside* the container (uses MYSQL_USER / MYSQL_PASS)
mysql:
	@$(COMPOSE) exec $(MYSQL_SERVICE) \
		mysql -u"$(MYSQL_USER)" -p"$(MYSQL_PASS)" -D "$(MYSQL_DATABASE)"

# Shell into the mysql container
sh:
	@$(COMPOSE) exec $(MYSQL_SERVICE) sh

# Quick ping (prints server version) via the container
ping:
	@$(COMPOSE) exec -T $(MYSQL_SERVICE) \
		mysql -u"$(MYSQL_USER)" -p"$(MYSQL_PASS)" -Nse "SELECT VERSION();"

# --- Migrations ---------------------------------------------------------------

# Apply every *.sql in MIGRATIONS_DIR in lexicographic order inside the container,
# but SKIP 1_init.sql because docker-entrypoint already executed /docker-entrypoint-initdb.d/1_init.sql
migrate:
	@echo "Applying migrations to $(MYSQL_DATABASE) via container '$(MYSQL_SERVICE)' as user '$(MYSQL_USER)'..."
	@set -euo pipefail; \
	if ! ls -1 "$(MIGRATIONS_DIR_ABS)"/*.sql >/dev/null 2>&1; then \
	  echo "No migration files found in $(MIGRATIONS_DIR_REL)."; \
	  exit 0; \
	fi; \
	for f in $$(ls -1 "$(MIGRATIONS_DIR_ABS)"/*.sql | sort); do \
	  case "$$f" in \
	    */1_init.sql) \
	      echo ">> Skipping $$f (already applied by docker-entrypoint-initdb.d)"; \
	      continue ;; \
	  esac; \
	  echo ">> $$f"; \
	  $(COMPOSE) exec -T $(MYSQL_SERVICE) \
	    mysql -u"$(MYSQL_USER)" -p"$(MYSQL_PASS)" "$(MYSQL_DATABASE)" < "$$f"; \
	done; \
	echo "✅ Migrations applied."

# DANGER: Drop & recreate database using ROOT, then migrate as MYSQL_USER
remigrate:
	@echo "!! DANGER: Dropping and recreating database '$(MYSQL_DATABASE)' as root !!"
	@read -p "Type 'yes' to continue: " ans; \
	if [ "$$ans" != "yes" ]; then echo "Aborted."; exit 1; fi; \
	$(COMPOSE) exec -T $(MYSQL_SERVICE) \
		mysql -uroot -p"$(MYSQL_ROOT_PASSWORD)" -e "DROP DATABASE IF EXISTS \`$(MYSQL_DATABASE)\`; CREATE DATABASE \`$(MYSQL_DATABASE)\` CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci;";
	@$(MAKE) migrate

# --- Sanity checks ------------------------------------------------------------

verify:
	@$(COMPOSE) exec -T $(MYSQL_SERVICE) \
		mysql -u"$(MYSQL_USER)" -p"$(MYSQL_PASS)" -D "$(MYSQL_DATABASE)" -e "SHOW TABLES;"
	@echo "Describe a key table (edit as needed):"
	@$(COMPOSE) exec -T $(MYSQL_SERVICE) \
		mysql -u"$(MYSQL_USER)" -p"$(MYSQL_PASS)" -D "$(MYSQL_DATABASE)" -e "DESCRIBE reviews;"

# --- Go project helpers -------------------------------------------------------

fmt:
	@go fmt ./...

lint:
	@echo "Configure your linter here (e.g., golangci-lint). Skipping for now."

test:
	@echo "Running unit tests (from $(ROOT_DIR))"
	@go test -C "$(ROOT_DIR)" -count=1 ./...

# Ensure tests can find migrations whether they look in internal/... or storage/...
ensure-migrations:
	@set -e; \
	if [ -d "$(MIGRATIONS_DIR_ABS)" ]; then \
	  mkdir -p "$(dir $(MIGRATIONS_MIRROR_ABS))"; \
	  rsync -a --delete "$(MIGRATIONS_DIR_ABS)/" "$(MIGRATIONS_MIRROR_ABS)/"; \
	  echo "Ensured migrations mirror at $(MIGRATIONS_MIRROR_REL)"; \
	else \
	  echo "WARNING: $(MIGRATIONS_DIR_REL) not found at $(ROOT_DIR)"; \
	fi

itest: ensure-migrations
	@echo "Running integration tests"
	@echo "ROOT_DIR=$(ROOT_DIR)"
	@echo "MIGRATIONS_DIR_ABS=$(MIGRATIONS_DIR_ABS) (mirror: $(MIGRATIONS_MIRROR_REL))"
	@MIGRATIONS_DIR="$(MIGRATIONS_DIR_ABS)" go test -C "$(ROOT_DIR)" -count=1 -tags=integration ./...

# --- Full reset & happy-path run ---------------------------------------------

# Drop containers AND volumes (clean slate)
nuke:
	@$(COMPOSE) down -v

# Rebuild images from scratch
rebuild:
	@$(COMPOSE) build --no-cache

# Wait until MySQL answers simple query
wait-mysql:
	@echo "Waiting for MySQL to be ready..."
	@until $(COMPOSE) exec -T $(MYSQL_SERVICE) \
		mysql -u"$(MYSQL_USER)" -p"$(MYSQL_PASS)" -e "SELECT 1" >/dev/null 2>&1; do \
		sleep 1; \
	done
	@echo "MySQL ready."

# One-shot: nuke -> rebuild images -> up -> wait mysql -> migrate -> ps
reset: nuke rebuild up wait-mysql migrate ps

# Run the ingestor interactively (ctrl+c to stop when done)
ingest:
	@$(COMPOSE) up ingestor
