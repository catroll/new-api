WEB_DIR = ./web
API_DIR = .
DEV_WEB_PORT ?= 5173
DEV_API_PORT ?= 3000
DEV_COMPOSE_FILE = docker-compose.dev.yml
DEV_POSTGRES_SERVICE = postgres
DEV_API_SERVICE = new-api
DEV_POSTGRES_DB = new-api
DEV_POSTGRES_USER = root
DEV_SQLITE_PATH ?= one-api.db

.PHONY: all build-web build-all-web start-api dev dev-api dev-api-rebuild dev-web stop reset-setup test

all: build-all-web start-api

build-web:
	@echo "Building web frontend..."
	@cd $(WEB_DIR) && bun install --frozen-lockfile
	@cd $(WEB_DIR) && DISABLE_ESLINT_PLUGIN='true' VITE_REACT_APP_VERSION=$$(cat ../VERSION) bun run build

build-all-web: build-web

start-api:
	@echo "Starting api dev server..."
	@cd $(API_DIR) && go run main.go &

dev-api:
	@echo "Starting api services (docker)..."
	@docker compose -f $(DEV_COMPOSE_FILE) up -d

dev-api-rebuild:
	@echo "Rebuilding and starting api service (docker)..."
	@docker compose -f $(DEV_COMPOSE_FILE) up -d --build $(DEV_API_SERVICE)

dev-web:
	@echo "Starting web frontend dev server..."
	@echo "Web frontend: http://localhost:$(DEV_WEB_PORT)"
	@cd $(WEB_DIR) && bun install
	@cd $(WEB_DIR) && bun run dev -- --host 0.0.0.0 --port $(DEV_WEB_PORT)

dev: dev-api dev-web

# Stop the local dev servers started by `start-api` and `dev-web`.
#
# Kill by listening port, not by process name: `start-api` runs `go run main.go`,
# which is only a wrapper. The process that actually binds the port is the
# compiled binary, either /tmp/go-build*/b001/exe/main or, with a warm build
# cache, <GOCACHE>/xx/<hash>-d/main - both carry "go-build" in the path. Killing
# the `go run` pid leaves that binary running as an orphan, still holding the
# port. Matching names is no better: `pkill -f exe/main` also matches the
# invoking shell, because the pattern sits in its own command line.
#
# Only pids that look like these two dev servers are signalled. The shipped
# docker-compose.yml also publishes :3000, and stopping that container by port
# would take the deployed site down, so a foreign listener is reported and left
# alone.
stop:
	@echo "Stopping local dev servers..."
	@for port in $(DEV_API_PORT) $(DEV_WEB_PORT); do \
		pids=""; \
		if command -v fuser >/dev/null 2>&1; then \
			pids=$$(fuser "$$port/tcp" 2>/dev/null | tr -s ' ' '\n' | grep -E '^[0-9]+$$'); \
		elif command -v lsof >/dev/null 2>&1; then \
			pids=$$(lsof -ti:"$$port" 2>/dev/null); \
		fi; \
		if [ -z "$$pids" ]; then echo "  port $$port: nothing listening"; continue; fi; \
		for pid in $$pids; do \
			cmd=$$(ps -p "$$pid" -o args= 2>/dev/null); \
			case "$$cmd" in \
				*go-build*|*rsbuild*dev*) \
					echo "  port $$port: stopping $$pid"; \
					kill -TERM "$$pid" 2>/dev/null || true ;; \
				*) \
					echo "  port $$port: skipped $$pid (not a local dev server): $$cmd" ;; \
			esac; \
		done; \
	done
	@echo "Done. Containers from 'make dev-api' are stopped with: docker compose -f $(DEV_COMPOSE_FILE) down"

# The main package embeds the ignored web/dist output and is covered after build-web.
test:
	@echo "Testing root Go module..."
	@root_module=$$(GOWORK=off go list -m); \
		root_packages=$$(GOWORK=off go list -e ./... | grep -vxF "$$root_module"); \
		GOWORK=off go test $$root_packages
	@echo "Testing relaykit Go module..."
	@cd relaykit && GOWORK=off go test ./...
	@echo "Testing tokenkit Go module..."
	@cd tokenkit && GOWORK=off go test ./...

reset-setup:
	@echo "Resetting local setup wizard state..."
	@if docker compose -f $(DEV_COMPOSE_FILE) ps --services --status running | grep -qx "$(DEV_POSTGRES_SERVICE)"; then \
		echo "Detected running docker dev PostgreSQL. Removing setup record and root users..."; \
		docker compose -f $(DEV_COMPOSE_FILE) exec -T $(DEV_POSTGRES_SERVICE) \
			psql -U $(DEV_POSTGRES_USER) -d $(DEV_POSTGRES_DB) \
			-c 'DELETE FROM setups;' \
			-c 'DELETE FROM users WHERE role = 100;' \
			-c "DELETE FROM options WHERE key IN ('SelfUseModeEnabled', 'DemoSiteEnabled');"; \
		echo "Restarting docker dev api so setup status is recalculated..."; \
		docker compose -f $(DEV_COMPOSE_FILE) restart $(DEV_API_SERVICE); \
	elif db_path="$${SQLITE_PATH:-$(DEV_SQLITE_PATH)}"; db_path="$${db_path%%\?*}"; [ -f "$$db_path" ]; then \
		db_path="$${SQLITE_PATH:-$(DEV_SQLITE_PATH)}"; \
		db_path="$${db_path%%\?*}"; \
		echo "Detected local SQLite database: $$db_path"; \
		sqlite3 "$$db_path" \
			"DELETE FROM setups; DELETE FROM users WHERE role = 100; DELETE FROM options WHERE key IN ('SelfUseModeEnabled', 'DemoSiteEnabled');"; \
		echo "SQLite setup state reset. Restart the local api process before testing the setup wizard."; \
	else \
		echo "No running docker dev PostgreSQL or local SQLite database found."; \
		echo "Start the dev stack with 'make dev-api', or set SQLITE_PATH/DEV_SQLITE_PATH to your local SQLite database."; \
		exit 1; \
	fi
