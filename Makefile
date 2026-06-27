.DEFAULT_GOAL := help
SHELL := /usr/bin/env bash

# ──────────────────────────────────────────────────────────────────────────
#  variables
# ──────────────────────────────────────────────────────────────────────────

VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -ldflags "-s -w -X github.com/undont/poke/internal/version.Version=$(VERSION)"
BIN     := bin
DIST    := dist
PLATFORMS := darwin/amd64 darwin/arm64 linux/amd64 linux/arm64

# colours
GREEN  := \033[0;32m
YELLOW := \033[0;33m
CYAN   := \033[0;36m
RED    := \033[0;31m
NC     := \033[0m

# log helpers
INFO := printf "$(CYAN)› %s$(NC)\n"
OK   := printf "$(GREEN)✓$(NC) %s\n"
WARN := printf "$(YELLOW)!$(NC) %s\n"
ERR  := printf "$(RED)✗ %s$(NC)\n"

.PHONY: help build install dist test test-race vet fmt lint tidy run-relay run-daemon clean

# ──────────────────────────────────────────────────────────────────────────
#  build
# ──────────────────────────────────────────────────────────────────────────

build: ## build poke and poked into bin/
	@$(INFO) "building $(VERSION)"
	@go build $(LDFLAGS) -o $(BIN)/poke ./cmd/poke
	@go build $(LDFLAGS) -o $(BIN)/poked ./cmd/poked
	@$(OK) "built $(BIN)/poke and $(BIN)/poked"

install: ## install poke and poked into the go bin dir
	@$(INFO) "installing $(VERSION)"
	@go install $(LDFLAGS) ./cmd/poke ./cmd/poked
	@$(OK) "installed to $$(go env GOBIN 2>/dev/null || echo $$(go env GOPATH)/bin)"

dist: ## cross-compile release binaries into dist/
	@$(INFO) "cross-compiling $(VERSION)"
	@rm -rf $(DIST) && mkdir -p $(DIST)
	@for p in $(PLATFORMS); do \
		os=$${p%/*}; arch=$${p#*/}; \
		for cmd in poke poked; do \
			out="$(DIST)/$${cmd}_$${os}_$${arch}"; \
			GOOS=$$os GOARCH=$$arch go build $(LDFLAGS) -o "$$out" ./cmd/$$cmd || exit 1; \
		done; \
		$(OK) "$$os/$$arch"; \
	done

# ──────────────────────────────────────────────────────────────────────────
#  quality
# ──────────────────────────────────────────────────────────────────────────

test: ## run all tests
	@go test ./...

test-race: ## run all tests with the race detector
	@go test -race ./...

vet: ## run go vet
	@go vet ./...

fmt: ## format the code with gofmt
	@gofmt -w .
	@$(OK) "formatted"

lint: ## check formatting and run go vet
	@out=$$(gofmt -l .); \
	if [ -n "$$out" ]; then $(ERR) "gofmt needed:"; echo "$$out"; exit 1; fi
	@go vet ./...
	@$(OK) "lint clean"

tidy: ## tidy go.mod and go.sum
	@go mod tidy
	@$(OK) "tidied"

# ──────────────────────────────────────────────────────────────────────────
#  run
# ──────────────────────────────────────────────────────────────────────────

run-relay: ## run the relay in the foreground
	@go run ./cmd/poked --relay

run-daemon: ## run the daemon in the foreground
	@go run ./cmd/poked

# ──────────────────────────────────────────────────────────────────────────
#  housekeeping
# ──────────────────────────────────────────────────────────────────────────

clean: ## remove build artefacts
	@rm -rf $(BIN) $(DIST)
	@$(OK) "cleaned"

help: ## show this help
	@cols=$$( { stty size </dev/tty; } 2>/dev/null | cut -d' ' -f2 ); \
	[ -n "$$cols" ] || cols=$$(tput cols 2>/dev/null); \
	case "$$cols" in ''|*[!0-9]*) cols=100;; esac; \
	[ "$$cols" -ge 40 ] || cols=100; \
	printf "\n  $(CYAN)poke$(NC) - terminal-native pokes for a small dev team\n\n"; \
	awk -v width="$$cols" ' \
		function wrap(text, w, ind,    n, words, i, line, out, pad) { \
			pad = sprintf("%" ind "s", ""); \
			n = split(text, words, " "); line = ""; out = ""; \
			for (i = 1; i <= n; i++) { \
				if (line == "") line = words[i]; \
				else if (length(line) + 1 + length(words[i]) <= w - ind) line = line " " words[i]; \
				else { out = out line "\n" pad; line = words[i]; } \
			} \
			return out line; \
		} \
		/^[a-zA-Z_-]+:.*## / { \
			split($$0, a, /:.*## /); \
			order[++cnt] = a[1] "\t" a[2]; \
			if (length(a[1]) > maxname) maxname = length(a[1]); \
		} \
		END { \
			ind = maxname + 6; \
			fmt = "  $(GREEN)%-" maxname "s$(NC)  %s\n"; \
			for (i = 1; i <= cnt; i++) { \
				split(order[i], p, "\t"); \
				printf fmt, p[1], wrap(p[2], width, ind); \
			} \
		} \
	' $(MAKEFILE_LIST); \
	printf "\n"
