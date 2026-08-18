# Recipe strict mode (fleet convention, Minecraft/custom-mods lineage): a
# failing or misspelled intermediate command aborts the recipe instead of
# being silently ignored.
SHELL := /bin/bash
.SHELLFLAGS := -eu -o pipefail -c

include .env.version

.PHONY: e2e e2e-ci
# Full supported matrix: the mc1.21.x jar's 1.20.3-1.20.6 floor releases (the
# T0 band — 1.20.3 flipped CommandManager.execute to void, this jar's hook
# shape, and the band IS its boundaries), every 1.21.x release, both 26.x
# minors, and the four sampled versions of the mc1.19-1.20.2 line.
# 26.1.1/26.1.2 are deliberately excluded from the default: the mc26.x jar
# covers >=26.1 <26.3 as one range, mapping breaks land on minor boundaries
# (both of which are covered), and each extra version costs a full server
# download. Run them explicitly when a 26.x patch is suspect:
#   make e2e VERSIONS="26.1 26.1.1 26.1.2 26.2"
# The 1.19.1-1.20.2 line (mc1192 jar, java floor 17) is sampled, not
# exhaustive, for the same reason: one jar covers >=1.19.1 <1.20.3 (1.19.0 is
# unsupported: its execute() lacks the ParseResults overload the jar hooks),
# and these four pin what matters - both ends of the 1.19 line, 1.20.1 (by far
# the most-run legacy version) and 1.20.2 (the boundary against 1.20.3, where
# execute() became void). Run the rest explicitly if suspect:
#   make e2e VERSIONS="1.19.1 1.19.3 1.20"
# The 1.14-1.18 line (mc114 jar, java floors 8/17) is sampled the same way:
# 1.16.5 and 1.18.2 are the two shipped niches, 1.17.1 the interior sample.
# 1.14.4 and 1.15.2 ride the identical jar and mixin; their one distinguishing
# property is the older `Recon` RCON source-name spelling, which the harness
# selects (and asserts exactly) from the version alone. Run them explicitly
# when the bottom of the range or the Recon path is suspect:
#   make e2e VERSIONS="1.14.4 1.15.2"
VERSIONS ?= 1.20.3 1.20.4 1.20.5 1.20.6 \
            1.21 1.21.1 1.21.2 1.21.3 1.21.4 1.21.5 1.21.6 1.21.7 1.21.8 \
            1.21.9 1.21.10 1.21.11 26.1 26.2 \
            1.19.2 1.19.4 1.20.1 1.20.2 \
            1.16.5 1.17.1 1.18.2

# `J` is accepted as a shorthand alias for `PARALLEL`. Neither has a Makefile
# default here on purpose: `e2e` and `e2e-ci` each supply their own default
# concurrency below, and an explicit `PARALLEL=`/`J=` on the command line must
# win over both. `origin` is how a Makefile tells "user passed this on the
# command line" apart from "nothing was set".
J ?=
_explicit_parallel := $(if $(filter command line,$(origin PARALLEL)),$(PARALLEL),$(if $(filter command line,$(origin J)),$(J),))

E2E_LOG_DIR := build/e2e-logs
E2E_RESULT_DIR := build/e2e-results
# Host-side cache of the per-version download artifacts (Fabric launcher +
# vanilla server jar), bind-mounted into every server container. Kills the
# 1.35GB the full matrix would otherwise re-download on every run (measured;
# see docs/superpowers/plans/2026-08-18-jar-download-cache.md). Disposable:
# `rm -rf` it any time. `make e2e E2E_JAR_CACHE=` disables it.
E2E_JAR_CACHE ?= $(HOME)/.cache/commandsspy-e2e-jars
E2E_RUN_ID := $(shell date +%Y%m%d-%H%M%S)-$$$$
BOOT_TIMEOUT ?= 180
MOD_JAR_121 := build/libs/commandsspy-$(MOD_VERSION)+mc1.21.x.jar
MOD_JAR_1192 := build/libs/commandsspy-$(MOD_VERSION)+mc1.19-1.20.2.jar
MOD_JAR_114 := build/libs/commandsspy-$(MOD_VERSION)+mc1.14.x.jar
MOD_JAR_26 := build/libs/commandsspy-$(MOD_VERSION)+mc26.x.jar

# Optional Java override applied to EVERY version in this run:
#   make e2e VERSIONS="1.21.11" JAVA=25
# Empty means "use each line's floor Java" (1.21.x -> 21, 26.x -> 25). The
# override exists because a Minecraft version has a Java FLOOR, not a Java pin:
# compatibility with newer JVMs has to be asserted, not assumed.
JAVA ?=

# Pinned Java runtimes, each verified to exist as eclipse-temurin:<n>-jre-jammy
# (all six tags checked 2026-08-17). Never add a number here without checking
# the tag resolves first. 11 is for MANUAL override runs only (frozen-Paper-era
# operators); it never appears in a default CI matrix.
JAVA_VERSIONS_SUPPORTED := 8 11 17 21 25 26

# Result/log keys. Without an override the key is the bare version, so existing
# filenames and CI artifacts are unchanged; with an override the Java version is
# part of the key so a grid run never overwrites another pair's log.
E2E_KEYS := $(if $(JAVA),$(addsuffix -java$(JAVA),$(VERSIONS)),$(VERSIONS))

# Pre-build the needed images SERIALLY. Two concurrent `docker build` calls
# writing the same tag race; doing it up front means the parallel phase only
# ever runs containers.
#
# Era-correct per-version Java floors (1.14-1.16.x -> 8, 1.17-1.20.2 -> 17,
# 1.20.3-1.21.x -> 21, 26.x -> 25) come from `scripts/e2e-run-one.sh
# --print-java <version>` — that script's case statement is the floor table's
# single home. 1.17.x's historical floor is Java 16, but no Temurin 16 jre
# image exists (only the EOL 16-jdk-focal), so it is CI-booted on 17; the
# jar's own `java >=8` guard still admits Java 16 operators.
# Offline probe of the default matrix, invoked by scripts/test-jar-routing.sh:
# the default VERSIONS list is part of the routing surface — a version dropped
# or added here silently changes what `make e2e` exercises.
.PHONY: print-e2e-versions
print-e2e-versions:
	@echo $(VERSIONS)

.PHONY: e2e-images
e2e-images:
	@mkdir -p $(E2E_LOG_DIR)
	@if [ -n "$(JAVA)" ] && ! echo "$(JAVA_VERSIONS_SUPPORTED)" | tr ' ' '\n' | grep -qx "$(JAVA)"; then \
	  echo "[e2e] Unsupported JAVA=$(JAVA). Supported: $(JAVA_VERSIONS_SUPPORTED)"; \
	  exit 1; \
	fi
	@if [ -n "$(JAVA)" ]; then \
	  javas="$(JAVA)"; \
	else \
	  javas=""; \
	  for version in $(VERSIONS); do \
	    jv=$$(./scripts/e2e-run-one.sh --print-java "$$version"); \
	    case " $$javas " in *" $$jv "*) ;; *) javas="$$javas $$jv" ;; esac; \
	  done; \
	fi; \
	for jv in $$javas; do \
	  echo "[e2e] Building image commandsspy-e2e:java$$jv..."; \
	  docker build --build-arg JAVA_VERSION=$$jv \
	    -t commandsspy-e2e:java$$jv . \
	    > $(E2E_LOG_DIR)/docker-build-java$$jv.log 2>&1 || { \
	    echo "[e2e] ✗ Docker build failed for java $$jv (see $(E2E_LOG_DIR)/docker-build-java$$jv.log)"; \
	    exit 1; \
	  }; \
	done

# `e2e` = local-dev command: fan out EVERY version's container at once (no
# bounded queue) for the fastest possible feedback. Default concurrency is the
# full version count; an explicit PARALLEL=/J= still wins.
e2e: clean-e2e $(MOD_JAR_121) $(MOD_JAR_1192) $(MOD_JAR_114) $(MOD_JAR_26) e2e-images
	@$(MAKE) _e2e-fanout VERSIONS="$(VERSIONS)" \
	  PARALLEL=$(if $(_explicit_parallel),$(_explicit_parallel),$(words $(VERSIONS)))

# `e2e-ci` = bounded variant for CI / resource-constrained runs: identical
# flow and machinery to `e2e`, but concurrency defaults to 4 instead of the
# full fan-out. CI matrix jobs call this as `make e2e-ci VERSIONS="<v>"`.
# gating (this default-4 cap) is deliberately NOT the local-dev behaviour.
e2e-ci: clean-e2e $(MOD_JAR_121) $(MOD_JAR_1192) $(MOD_JAR_114) $(MOD_JAR_26) e2e-images
	@$(MAKE) _e2e-fanout VERSIONS="$(VERSIONS)" \
	  PARALLEL=$(if $(_explicit_parallel),$(_explicit_parallel),4)

# Shared core: fan out, reap result files, print the summary. Both `e2e` and
# `e2e-ci` invoke this via a sub-make with PARALLEL already resolved, so this
# target is not meant to be called directly.
.PHONY: _e2e-fanout
_e2e-fanout:
	@echo "[e2e] Testing Minecraft versions: $(VERSIONS)"
	@echo "[e2e] Java: $(if $(JAVA),$(JAVA) (override),per-version era floor (1.14-1.16=8, 1.17-1.20.2=17, 1.20.3-1.21.x=21, 26.x=25))"
	@echo "[e2e] Concurrency: $(PARALLEL)"
	@mkdir -p $(E2E_LOG_DIR) $(E2E_RESULT_DIR)
	@printf '%s\n' $(VERSIONS) | \
	  REPO_ROOT="$(PWD)" \
	  MOD_JAR_121="$(MOD_JAR_121)" \
	  MOD_JAR_1192="$(MOD_JAR_1192)" \
	  MOD_JAR_114="$(MOD_JAR_114)" \
	  MOD_JAR_26="$(MOD_JAR_26)" \
	  E2E_LOG_DIR="$(E2E_LOG_DIR)" \
	  E2E_RESULT_DIR="$(E2E_RESULT_DIR)" \
	  E2E_RUN_ID="$(E2E_RUN_ID)" \
	  BOOT_TIMEOUT="$(BOOT_TIMEOUT)" \
	  JAVA_OVERRIDE="$(JAVA)" \
	  E2E_JAR_CACHE="$(E2E_JAR_CACHE)" \
	  xargs -P $(PARALLEL) -n 1 ./scripts/e2e-run-one.sh || true
	@echo ""
	@echo "[e2e] Summary:"
	@failed=0; \
	for key in $(E2E_KEYS); do \
	  result_file="$(E2E_RESULT_DIR)/$$key.result"; \
	  if [ -f "$$result_file" ]; then \
	    line=$$(head -1 "$$result_file"); \
	  else \
	    line="E2E $$key FAIL no-result"; \
	  fi; \
	  echo "  $$line"; \
	  case "$$line" in \
	    *" PASS"|*" PASS players-skipped-unsupported-protocol") ;; \
	    *) failed=1 ;; \
	  esac; \
	done; \
	if [ $$failed -eq 1 ]; then \
	  echo "[e2e] Some versions failed. Logs retained in $(E2E_LOG_DIR)/"; \
	  exit 1; \
	fi; \
	echo "[e2e] All versions passed!"; \
	echo "[e2e] Logs saved to $(E2E_LOG_DIR)/"

.PHONY: clean-e2e
clean-e2e:
	@rm -rf $(E2E_LOG_DIR) $(E2E_RESULT_DIR)
	@docker ps -aq --filter "label=commandsspy-e2e=1" 2>/dev/null | xargs -r docker rm -f > /dev/null 2>&1 || true
	@docker ps -aq --filter "name=commandsspy-e2e-" 2>/dev/null | xargs -r docker rm -f > /dev/null 2>&1 || true
	@docker images -q "commandsspy-e2e:*" 2>/dev/null | xargs -r docker rmi -f > /dev/null 2>&1 || true

# Gradle cannot run two builds concurrently in one project directory, and all
# real e2e parallelism happens inside the e2e recipe (xargs -P), so serializing
# Make's own recipes costs nothing and makes `make -j` safe.
.NOTPARALLEL:

$(MOD_JAR_121):
	@echo "[build] Building 1.21.x jar..."
	@./gradlew build --no-daemon --quiet

$(MOD_JAR_1192):
	@echo "[build] Building 1.19-1.20.2 jar..."
	@./gradlew build -PmcTarget=1192 --no-daemon --quiet

$(MOD_JAR_114):
	@echo "[build] Building 1.14.x jar (Minecraft 1.14-1.18)..."
	@./gradlew build -PmcTarget=114 --no-daemon --quiet

$(MOD_JAR_26):
	@echo "[build] Building 26.x jar..."
	@./gradlew build -PmcTarget=26 --no-daemon --quiet

.PHONY: build
build: $(MOD_JAR_121) $(MOD_JAR_1192) $(MOD_JAR_114) $(MOD_JAR_26)

# Unit tests for the shared, mapping-agnostic command handler. ALL targets are run
# because each resolves its own fabric-loader / fabric-loader-junit line (0.16.5 vs
# 0.19.3) and its own compile level (release 17 vs 21, toolchain 21 vs 25) - a suite
# that is green on one can still fail on another, which is precisely the class of
# breakage this repo cares about.
# Unlike the jar rules this is .PHONY: there is no output file to compare timestamps
# against, and re-running the tests is the point.
.PHONY: test
test:
	@echo "[test] Offline version->jar routing contract..."
	@scripts/test-jar-routing.sh
	@echo "[test] Unit tests (1.21.x target)..."
	@./gradlew test --no-daemon
	@echo "[test] Unit tests (1.19-1.20.2 target)..."
	@./gradlew test -PmcTarget=1192 --no-daemon
	@echo "[test] Unit tests (1.14-1.18 target)..."
	@./gradlew test -PmcTarget=114 --no-daemon
	@echo "[test] Unit tests (26.x target)..."
	@./gradlew test -PmcTarget=26 --no-daemon
	@echo "[test] All targets passed."

# ---------------------------------------------------------------------------
# Fast quality gate. `make ci` is the ONE gate: the pre-commit hook and the CI
# go-tools job both run exactly this target (fleet convention, AM5/cpu-ram-test
# lineage). Cheap checks first: shell syntax fails in milliseconds, not after
# a lint/build/test ladder. Deliberately excludes e2e (docker) and the Gradle
# build/test - those are the slow, separate gates.
#
# Lint tools are pinned by version and run via `go run tool@version`: the pin
# lives in one variable, with zero install or version-drift machinery.
GOLANGCI_LINT_VERSION := 2.12.2
GOVULNCHECK_VERSION := 1.7.0

# Self-adjusts as scripts appear/disappear; enumerates nothing by name.
SH_SOURCES := $(shell git ls-files '*.sh')

.PHONY: lint-sh
lint-sh: ## bash -n + shellcheck over every tracked *.sh
	@for f in $(SH_SOURCES); do echo "bash -n $$f"; bash -n "$$f" || exit 1; done
	@test -z "$(SH_SOURCES)" && exit 0; \
	 command -v shellcheck >/dev/null || { echo "shellcheck missing (apt/brew install shellcheck)"; exit 1; }; \
	 shellcheck -S style $(SH_SOURCES)

.PHONY: go-fmt go-fmt-check go-vet go-lint go-vuln go-build go-test
go-fmt:
	cd tools && gofmt -w .

go-fmt-check: ## report-only: CI must be able to fail (fleet rule)
	@unformatted="$$(cd tools && gofmt -l .)"; \
	 if [ -n "$$unformatted" ]; then echo "gofmt needed on:" >&2; echo "$$unformatted" >&2; exit 1; fi

go-vet:
	cd tools && go vet ./...

go-lint:
	cd tools && go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v$(GOLANGCI_LINT_VERSION) run

go-vuln:
	cd tools && go run golang.org/x/vuln/cmd/govulncheck@v$(GOVULNCHECK_VERSION) ./...

go-build: ## the static binary is the deliverable - prove it every build
	cd tools && CGO_ENABLED=0 go build -trimpath -o /dev/null .

go-test:
	cd tools && go test -race -count=1 -cover ./...

.PHONY: ci
ci: lint-sh go-fmt-check go-vet go-lint go-build go-test go-vuln ## the one fast gate; hook and CI both run this

.PHONY: hooks
hooks: ## arm the tracked pre-commit hook
	git config core.hooksPath .githooks

# Report the server-reported boot time for each version from its e2e log.
# `Done (12.345s)!` is emitted by the Minecraft server itself, so this measures
# server boot, not download or container overhead.
.PHONY: e2e-times
e2e-times:
	@for log in $(E2E_LOG_DIR)/*.log; do \
	  case "$$log" in *docker-build-*) continue ;; esac; \
	  version=$$(basename "$$log" .log); \
	  done_line=$$(grep -oE 'Done \([0-9.]+s\)' "$$log" | tail -1 || true); \
	  printf '%-10s %s\n' "$$version" "$${done_line:-<no Done line>}"; \
	done
