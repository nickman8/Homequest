GO ?= go
WIDGET_SANDBOX ?= $(CURDIR)/.tools/widget-sandbox
.PHONY: build test widgets harness migrate dev preview
build:
	mkdir -p bin
	$(GO) build -o bin/donetick .
	$(GO) build -o bin/widget-probe ./cmd/widget-probe
	$(GO) build -o bin/bot-probe ./cmd/bot-probe
	$(GO) build -o bin/homequest ./cmd/homequest
	$(GO) build -o bin/homequest-migrate ./cmd/homequest-migrate
test:
	$(GO) test ./...
widgets:
	npm ci --ignore-scripts
	npm run build:widgets
harness: build widgets
	WIDGET_SANDBOX=$(WIDGET_SANDBOX) WIDGET_PROBE=$(CURDIR)/bin/widget-probe $(WIDGET_SANDBOX)/node_modules/.bin/tsx tests/widget-harness.mts
	WIDGET_SANDBOX=$(WIDGET_SANDBOX) $(WIDGET_SANDBOX)/node_modules/.bin/tsx tests/bot-widget-e02.mts
	WIDGET_SANDBOX=$(WIDGET_SANDBOX) $(WIDGET_SANDBOX)/node_modules/.bin/tsx tests/widget-feedback.mts
	mkdir -p .tools
	HQ_WIDGET_CARDS_OUT=$(CURDIR)/.tools/widget-cards.json $(GO) test ./internal/homequest/bot -run TestCatalogs -count=1
	HQ_FEATURE_CARDS_OUT=$(CURDIR)/.tools/widget-feature-cards.json $(GO) test ./internal/homequest/bot -run TestFeatureCardBudgets -count=1
	WIDGET_SANDBOX=$(WIDGET_SANDBOX) HQ_WIDGET_CARDS_OUT=$(CURDIR)/.tools/widget-cards.json HQ_FEATURE_CARDS_OUT=$(CURDIR)/.tools/widget-feature-cards.json $(WIDGET_SANDBOX)/node_modules/.bin/tsx tests/card-layouts.mts
	WIDGET_SANDBOX=$(WIDGET_SANDBOX) $(WIDGET_SANDBOX)/node_modules/.bin/tsx tests/preview.mts
	WIDGET_SANDBOX=$(WIDGET_SANDBOX) $(WIDGET_SANDBOX)/node_modules/.bin/tsx tests/household-e2e.mts
	WIDGET_SANDBOX=$(WIDGET_SANDBOX) $(WIDGET_SANDBOX)/node_modules/.bin/tsx tests/plans-e2e.mts
	WIDGET_SANDBOX=$(WIDGET_SANDBOX) $(WIDGET_SANDBOX)/node_modules/.bin/tsx tests/shopping-e2e.mts
	WIDGET_SANDBOX=$(WIDGET_SANDBOX) $(WIDGET_SANDBOX)/node_modules/.bin/tsx tests/assistant-e2e.mts
	WIDGET_SANDBOX=$(WIDGET_SANDBOX) $(WIDGET_SANDBOX)/node_modules/.bin/tsx tests/e2e-runtime.mts
dev: build
	./scripts/dev.sh
migrate:
	$(GO) run ./cmd/homequest-migrate
preview: build widgets
	node scripts/preview-server.mjs
