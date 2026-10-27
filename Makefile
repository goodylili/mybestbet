FRONTEND := mybestbet-frontend
BACKEND := mybestbet-backend
PM := $(shell if [ -f $(FRONTEND)/pnpm-lock.yaml ]; then echo pnpm; elif [ -f $(FRONTEND)/yarn.lock ]; then echo yarn; elif [ -f $(FRONTEND)/bun.lockb ] || [ -f $(FRONTEND)/bun.lock ]; then echo bun; else echo npm; fi)
INSTALL := $(if $(filter npm,$(PM)),npm ci,$(if $(filter yarn,$(PM)),yarn install --frozen-lockfile,$(PM) install --frozen-lockfile))

COMPOSE := docker compose -f deploy/docker-compose.yml -f deploy/docker-compose.local.yml

.PHONY: ci backend-ci frontend-ci infra up down logs \
	frontend-install frontend-dev frontend-lint frontend-typecheck frontend-build frontend-start frontend-clean

ci: backend-ci frontend-ci

backend-ci:
	$(MAKE) -C $(BACKEND) ci

frontend-ci:
	@if [ ! -f $(FRONTEND)/package.json ]; then echo "$(FRONTEND): no package.json, skipped"; exit 0; fi; \
	cd $(FRONTEND) && $(INSTALL) && $(PM) run --if-present lint && $(PM) run --if-present typecheck && $(PM) run build

frontend-install:
	cd $(FRONTEND) && $(INSTALL)

frontend-dev:
	cd $(FRONTEND) && $(PM) run dev

frontend-lint:
	cd $(FRONTEND) && $(PM) run --if-present lint

frontend-typecheck:
	cd $(FRONTEND) && $(PM) run --if-present typecheck

frontend-build:
	cd $(FRONTEND) && $(PM) run build

frontend-start:
	cd $(FRONTEND) && $(PM) run start

frontend-clean:
	rm -rf $(FRONTEND)/.next $(FRONTEND)/node_modules $(FRONTEND)/tsconfig.tsbuildinfo

infra:
	$(MAKE) -C $(BACKEND) infra

up:
	$(COMPOSE) up --build --watch

down:
	$(COMPOSE) down

logs:
	$(COMPOSE) logs -f --tail=100
