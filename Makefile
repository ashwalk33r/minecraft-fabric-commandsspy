# Strict recipes: a failing intermediate command aborts.
SHELL := /bin/bash
.SHELLFLAGS := -eu -o pipefail -c

include .env.version

# Bare `make` prints the target catalog, never runs e2e.
.DEFAULT_GOAL := help
.PHONY: help
help: ## list every documented target with its description
	@grep -hE '^[a-zA-Z0-9_-]+:.*##' $(MAKEFILE_LIST) | sed -E 's/^([a-zA-Z0-9_-]+):[^#]*##[[:space:]]?/\1\t/' | sort | awk -F'\t' '{printf "  \033[1m%-20s\033[0m %s\n", $$1, $$2}'


.PHONY: e2e e2e-ci
# Default matrix: sampled per band — rationale in the wiki, Supported-Versions
# -> "The Makefile's default version list".
# Suspect versions can be run explicitly, e.g.:
#   make e2e VERSIONS="26.1 26.1.1 26.1.2 26.2"
#   make e2e VERSIONS="1.19.1 1.19.3 1.20 1.14.4 1.15.2"
VERSIONS ?= 1.20.3 1.20.4 1.20.5 1.20.6 \
            1.21 1.21.1 1.21.2 1.21.3 1.21.4 1.21.5 1.21.6 1.21.7 1.21.8 \
            1.21.9 1.21.10 1.21.11 26.1 26.2 \
            1.19.2 1.19.4 1.20.1 1.20.2 \
            1.16.5 1.17.1 1.18.2

# J = alias for PARALLEL. No default here so a command-line value wins over
# each target's own default (origin tells them apart).
J ?=
_explicit_parallel := $(if $(filter command line,$(origin PARALLEL)),$(PARALLEL),$(if $(filter command line,$(origin J)),$(J),))

E2E_LOG_DIR := build/e2e-logs
E2E_RESULT_DIR := build/e2e-results
# Host-side cache of per-version download artifacts; disposable.
# E2E_JAR_CACHE= disables it. See docs/e2e-harness.md.
E2E_JAR_CACHE ?= $(HOME)/.cache/commandsspy-e2e-jars
E2E_RUN_ID := $(shell date +%Y%m%d-%H%M%S)-$$$$
BOOT_TIMEOUT ?= 180
MOD_JAR_121 := build/libs/commandsspy-$(MOD_VERSION)+mc1.21.x.jar
MOD_JAR_1192 := build/libs/commandsspy-$(MOD_VERSION)+mc1.19-1.20.2.jar
MOD_JAR_114 := build/libs/commandsspy-$(MOD_VERSION)+mc1.14.x.jar
MOD_JAR_26 := build/libs/commandsspy-$(MOD_VERSION)+mc26.x.jar
# Four Forge jars, built by the separate forge/ Gradle build via -PforgeTarget
# (default 'modern'). See the wiki, Supported-Versions -> "Forge".
MOD_JAR_FORGE := forge/build/libs/commandsspy-$(MOD_VERSION)+mc1.21.x-forge.jar
MOD_JAR_FORGE_LEGACY := forge/build/libs/commandsspy-$(MOD_VERSION)+mc1.17-1.20.4-forge.jar
MOD_JAR_FORGE_EB7 := forge/build/libs/commandsspy-$(MOD_VERSION)+mc1.21.6-26.2-forge.jar
MOD_JAR_FORGE_MC116 := forge/build/libs/commandsspy-$(MOD_VERSION)+mc1.16.x-forge.jar
# NeoForge band jar: ONE jar spanning the measured range, built by the
# standalone neoforge/ Gradle build. NeoForge has no SRG era, so the range is
# bounded by what booted, not by mappings; see the wiki,
# Version-Boundaries-And-Root-Causes -> "Why one jar spans the whole NeoForge history".
MOD_JAR_NEO := build/libs/commandsspy-$(MOD_VERSION)+mc1.20.2-26.2-neoforge.jar

# Optional Java override applied to EVERY version in this run:
#   make e2e VERSIONS="1.21.11" JAVA=25
# Empty = each version's floor. A version has a Java floor, not a pin -- with
# one exception: the Forge modern band (1.20.6-1.21.5) is java 21 ONLY, and a
# higher JVM is refused with above-java-ceiling-21 (issue #66).
JAVA ?=

# Each must exist as eclipse-temurin:<n>-jre-jammy; check the tag before
# adding. 11 is manual-override only.
JAVA_VERSIONS_SUPPORTED := 8 17 21 25 26

# fabric (default) | quilt | forge | neoforge — which loader's server boots.
# See docs/e2e-harness.md.
LOADER ?= fabric

# Key = version[-<loader>][-java<N>], mirroring scripts/e2e-run-one.sh's own KEY
# construction, so grid runs never collide across loaders or Java overrides.
# fabric is the default and contributes no suffix, so its keys (and therefore
# its logs, results and container names) are byte-for-byte what they were before
# the loader axis existed.
_loader_suffix := $(if $(filter fabric,$(LOADER)),,-$(LOADER))

# 1 = run the config-behaviors e2e leg (pre-seeded blacklist + logArguments:true)
# instead of the default assertions. Mirrors scripts/e2e-run-one.sh's own
# CONFIG_VARIANT and its "-cfgvar" KEY suffix. See docs/e2e-harness.md.
CONFIG_VARIANT ?= 0
_cfgvar_suffix := $(if $(filter 1,$(CONFIG_VARIANT)),-cfgvar,)

# 1 = the out-of-range refusal guard leg: run a Minecraft version that NO
# declared minecraft_range_* covers and assert the Fabric/Quilt loader refuses
# the mod. Read from the environment by scripts/e2e-run-one.sh; declared here so
# `make e2e FABRIC_EXPECT_REFUSED=1` works alongside the env form. No KEY suffix:
# the guard version (1.19.0) is never in the default matrix, so it cannot
# collide. See docs/e2e-harness.md.
FABRIC_EXPECT_REFUSED ?= 0

# 1 = the Forge out-of-range refusal guard leg. Forge's analogue of the leg
# above cannot be built the same way: the two holes between the four declared
# Forge ranges are 1.17 and 1.20.5, and Forge published no server build for
# either -- the holes exist BECAUSE nothing was published there -- so a run on
# them dies at no-forge-build-for-version before a container ever starts. What
# is bootable is the same proposition with the mismatch on the other axis:
# hand a version Forge DOES publish for the WRONG band's jar, by pre-setting
# scripts/e2e-run-one.sh's FORGE_JAR_BAND instead of letting it route by
# version. `modern` is the only band that isolates the minecraft range: every
# band's loader_range mirrors its minecraft_range (Forge's major tracks the
# Minecraft version 1:1), so on any other pairing the loaderVersion gate
# refuses first and the minecraft range is never evaluated. modern's
# loader_range is [50,) -- unbounded above -- so on 1.21.6 the javafml and
# forge gates both pass and only minecraft_range_modern's <1.21.6 ceiling
# refuses. FORGE_EXPECT_REFUSED then falls out for free: 1.21.6 is not in
# FORGE_KNOWN_GOOD_MODERN, so the routing table raises the flag itself.
FORGE_REFUSAL_PROBE ?= 0
_forge_jar_band := $(FORGE_JAR_BAND)
ifeq ($(FORGE_REFUSAL_PROBE),1)
_forge_jar_band := modern
endif

# Only LOADER=forge needs the Forge jars built; a Fabric/Quilt/NeoForge run
# must not pay for ForgeGradle's decompile pipeline. All four Forge jars are
# built for any forge e2e run -- scripts/e2e-run-one.sh routes per version
# (see its VERSION case statement), and a single run's VERSIONS list can mix
# mc116, legacy, modern and eventbus7 versions.
_forge_jar_dep := $(if $(filter forge,$(LOADER)),$(MOD_JAR_FORGE) $(MOD_JAR_FORGE_LEGACY) $(MOD_JAR_FORGE_EB7) $(MOD_JAR_FORGE_MC116),)

# Only LOADER=neoforge needs the NeoForge jar built; a Fabric, Quilt or Forge
# run must not pay for ModDevGradle's NeoForm pipeline.
_neo_jars := $(if $(filter neoforge,$(LOADER)),$(MOD_JAR_NEO),)
E2E_KEYS := $(addsuffix $(_cfgvar_suffix),$(if $(JAVA),$(addsuffix -java$(JAVA),$(addsuffix $(_loader_suffix),$(VERSIONS))),$(addsuffix $(_loader_suffix),$(VERSIONS))))

# Pre-build the needed images SERIALLY: two concurrent `docker build` calls
# writing the same tag race, so the parallel phase only ever runs containers.
# Floors come from `scripts/e2e-run-one.sh --print-java` — the table's single
# home. See the wiki, Supported-Versions -> "Java floors".
# Offline probe used by scripts/test-jar-routing.sh: the default list is part
# of the routing surface.
.PHONY: print-e2e-versions
print-e2e-versions: ## print the default e2e version matrix (routing-test probe)
	@echo $(VERSIONS)

# Tag is content-addressed over everything the image depends on: the
# Dockerfile itself, the entrypoint script it COPYs, and every tracked file
# under tools/ (what the build stage compiles). Published to GHCR alongside
# CI_IMAGE (repo is public: free, unlimited storage/bandwidth) so e2e-images
# below can `docker pull` instead of a full `docker build` on a fresh CI
# runner -- measured ~85%/~73% faster for java21/java8 respectively (cold
# `docker build` vs. cold pull of an equivalent pre-built image; see
# docs/ci.md). Not a byte-identical guarantee -- apt-get installs whatever
# curl point release is current, unpinned -- but that's the same tradeoff
# CI_IMAGE already accepts for its own apt layer.
#
# The alpine-vs-jammy variant choice below is NOT itself part of this hash
# (it lives in this Makefile, not in the hashed files) -- it's baked into
# the tag string instead ("java$$jv-$$variant-$(E2E_IMAGE_HASH)"), so
# changing which floors get alpine still busts the right tags instead of
# silently serving a stale wrong-variant image under an unchanged hash.
E2E_IMAGE_BASE := ghcr.io/ashwalk33r/commandsspy-e2e
E2E_IMAGE_HASH := $(shell git ls-files Dockerfile scripts/e2e-entrypoint.sh tools/ | sort | xargs cat | git hash-object --stdin | cut -c1-12)

.PHONY: e2e-images
e2e-images: ## pull-or-build the per-Java server Docker images serially, tag locally
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
	  case "$$jv" in \
	    21|25|26) variant=alpine ;; \
	    *)        variant=jammy ;; \
	  esac; \
	  local_tag="commandsspy-e2e:java$$jv"; \
	  ghcr_tag="$(E2E_IMAGE_BASE):java$$jv-$$variant-$(E2E_IMAGE_HASH)"; \
	  if docker image inspect "$$ghcr_tag" > /dev/null 2>&1; then \
	    echo "[e2e] $$ghcr_tag already present locally."; \
	  elif docker pull "$$ghcr_tag" > /dev/null 2>&1; then \
	    echo "[e2e] Pulled $$ghcr_tag from GHCR."; \
	  else \
	    echo "[e2e] $$ghcr_tag not found locally or on GHCR; building $$local_tag (base: $$variant)..."; \
	    docker build --build-arg JAVA_VERSION=$$jv --build-arg BASE_VARIANT=$$variant \
	      -t "$$ghcr_tag" . \
	      2>&1 | tee $(E2E_LOG_DIR)/docker-build-java$$jv.log | sed -u "s/^/[java$$jv] /" || { \
	      echo "[e2e] ✗ Docker build failed for java $$jv (see $(E2E_LOG_DIR)/docker-build-java$$jv.log)"; \
	      exit 1; \
	    }; \
	    if [ -n "$${CI:-}" ]; then \
	      echo "[e2e] Publishing $$ghcr_tag to GHCR..."; \
	      docker push "$$ghcr_tag" || echo "[e2e] Push failed (non-fatal, this job still has the image locally)"; \
	    fi; \
	  fi; \
	  docker tag "$$ghcr_tag" "$$local_tag"; \
	done

e2e: clean-e2e $(MOD_JAR_121) $(MOD_JAR_1192) $(MOD_JAR_114) $(MOD_JAR_26) $(_forge_jar_dep) $(_neo_jars) e2e-images ## full e2e: boot every version in VERSIONS, max parallel, console+RCON+player asserts
	@$(MAKE) _e2e-fanout VERSIONS="$(VERSIONS)" \
	  PARALLEL=$(if $(_explicit_parallel),$(_explicit_parallel),$(words $(VERSIONS)))

e2e-ci: clean-e2e $(MOD_JAR_121) $(MOD_JAR_1192) $(MOD_JAR_114) $(MOD_JAR_26) $(_forge_jar_dep) $(_neo_jars) e2e-images ## bounded e2e for CI/constrained runs (PARALLEL=4 default), same assertions
	@$(MAKE) _e2e-fanout VERSIONS="$(VERSIONS)" \
	  PARALLEL=$(if $(_explicit_parallel),$(_explicit_parallel),4)

# Internal: callers resolve PARALLEL first.
.PHONY: _e2e-fanout
_e2e-fanout:
	@case "$(LOADER)" in \
	  fabric|quilt|forge|neoforge) ;; \
	  *) echo "[e2e] Unsupported LOADER=$(LOADER). Supported: fabric quilt forge neoforge"; exit 1 ;; \
	esac
	@echo "[e2e] Testing Minecraft versions: $(VERSIONS)"
	@echo "[e2e] Loader: $(LOADER)"
	@echo "[e2e] Java: $(if $(JAVA),$(JAVA) (override),per-version era floor (1.14-1.16=8, 1.17-1.20.2=17, 1.20.3-1.21.x=21, 26.x=25))"
	@if [ "$(LOADER)" = "forge" ]; then echo "[e2e] Note: the Forge modern band (1.20.6-1.21.5) is java 21 only (issue #66)"; fi
	@echo "[e2e] Concurrency: $(PARALLEL)"
	@mkdir -p $(E2E_LOG_DIR) $(E2E_RESULT_DIR)
	@printf '%s\n' $(VERSIONS) | \
	  REPO_ROOT="$(PWD)" \
	  MOD_JAR_121="$(MOD_JAR_121)" \
	  MOD_JAR_1192="$(MOD_JAR_1192)" \
	  MOD_JAR_114="$(MOD_JAR_114)" \
	  MOD_JAR_26="$(MOD_JAR_26)" \
	  MOD_JAR_FORGE="$(MOD_JAR_FORGE)" \
	  MOD_JAR_FORGE_LEGACY="$(MOD_JAR_FORGE_LEGACY)" \
	  MOD_JAR_FORGE_EB7="$(MOD_JAR_FORGE_EB7)" \
	  MOD_JAR_FORGE_MC116="$(MOD_JAR_FORGE_MC116)" \
	  MOD_JAR_NEO="$(MOD_JAR_NEO)" \
	  E2E_LOG_DIR="$(E2E_LOG_DIR)" \
	  E2E_RESULT_DIR="$(E2E_RESULT_DIR)" \
	  E2E_RUN_ID="$(E2E_RUN_ID)" \
	  BOOT_TIMEOUT="$(BOOT_TIMEOUT)" \
	  JAVA_OVERRIDE="$(JAVA)" \
	  LOADER="$(LOADER)" \
	  CONFIG_VARIANT="$(CONFIG_VARIANT)" \
	  FORGE_JAR_BAND="$(_forge_jar_band)" \
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
	    *" PASS"|*" PASS players-skipped-unsupported-protocol"|*" PASS forge-out-of-range-refused-as-expected"|*" PASS fabric-out-of-range-refused-as-expected"|*" PASS config-behaviors") ;; \
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
clean-e2e: ## remove e2e logs/results and reap containers/images
	@rm -rf $(E2E_LOG_DIR) $(E2E_RESULT_DIR)
	@docker ps -aq --filter "label=commandsspy-e2e=1" 2>/dev/null | xargs -r docker rm -f > /dev/null 2>&1 || true
	@docker ps -aq --filter "name=commandsspy-e2e-" 2>/dev/null | xargs -r docker rm -f > /dev/null 2>&1 || true
	@docker images -q "commandsspy-e2e:*" 2>/dev/null | xargs -r docker rmi -f > /dev/null 2>&1 || true

# Gradle cannot run two builds concurrently in one project directory; e2e
# parallelism lives inside the recipe (xargs -P), so `make -j` is safely serialized.
.NOTPARALLEL:

MOD_SOURCES := $(shell git ls-files src neoforge '*.gradle' gradle.properties .env.version)

$(MOD_JAR_121): $(MOD_SOURCES) | ci-image
	@echo "[build] Building 1.21.x jar..."
	@$(call in_ci_image_gradle,gradle build --no-daemon --quiet)

$(MOD_JAR_1192): $(MOD_SOURCES) | ci-image
	@echo "[build] Building 1.19-1.20.2 jar..."
	@$(call in_ci_image_gradle,gradle build -PmcTarget=1192 --no-daemon --quiet)

$(MOD_JAR_114): $(MOD_SOURCES) | ci-image
	@echo "[build] Building 1.14.x jar (Minecraft 1.14-1.18)..."
	@$(call in_ci_image_gradle,gradle build -PmcTarget=114 --no-daemon --quiet)

$(MOD_JAR_26): $(MOD_SOURCES) | ci-image
	@echo "[build] Building 26.x jar..."
	@$(call in_ci_image_gradle,gradle build -PmcTarget=26 --no-daemon --quiet)

# Deliberately NOT a dependency of `build`: Forge jars decompile Minecraft on
# their first build each (~5 min, ~1.2G of ForgeGradle cache under
# GRADLE_USER_HOME, per -PforgeTarget). Built on demand and by the LOADER=forge
# e2e legs. `-p forge` runs the separate forge/ build; same pinned image as
# everything else. `build-forge` (no args, default -PforgeTarget=modern)
# builds ONLY the modern jar, unchanged from Phase-1.
$(MOD_JAR_FORGE): $(shell git ls-files forge src/main/java .env.version) | ci-image
	@echo "[build] Building Forge jar (MC 1.21.1, Forge 52.1.x)..."
	@$(call in_ci_image_gradle,gradle -p forge build -PforgeTarget=modern --no-daemon --quiet)

.PHONY: build-forge
build-forge: $(MOD_JAR_FORGE) ## build the modern Forge jar, MC 1.20.6-1.21.5 (host gradlew + ForgeGradle 7)

# Legacy jar: issue #28 task 1, SRG-reobfuscated for the pre-1.20.5 era.
$(MOD_JAR_FORGE_LEGACY): $(shell git ls-files forge src/main/java .env.version) | ci-image
	@echo "[build] Building legacy Forge jar (MC 1.20.1 compile target, Forge 47.4.x, SRG-renamed)..."
	@$(call in_ci_image_gradle,gradle -p forge build -PforgeTarget=legacy --no-daemon --quiet)

.PHONY: build-forge-legacy
build-forge-legacy: $(MOD_JAR_FORGE_LEGACY) ## build the legacy Forge jar, MC 1.17.1-1.20.4, SRG-reobfuscated (host gradlew + ForgeGradle 7)

# EventBus-7 jar: issue #32 task 2, for the Forge 56+ era above the modern
# jar's <1.21.6 ceiling.
$(MOD_JAR_FORGE_EB7): $(shell git ls-files forge src/main/java .env.version) | ci-image
	@echo "[build] Building EventBus-7 Forge jar (MC 1.21.8 compile target, Forge 58.1.x)..."
	@$(call in_ci_image_gradle,gradle -p forge build -PforgeTarget=eventbus7 --no-daemon --quiet)

.PHONY: build-forge-eventbus7
build-forge-eventbus7: $(MOD_JAR_FORGE_EB7) ## build the EventBus-7 Forge jar, MC 1.21.6-26.2 (host gradlew + ForgeGradle 7)

# mc116 jar: issue #30, SRG-reobfuscated for the pre-1.17 era (its own
# entrypoint source too, compiled against pre-1.17 class names -- see
# forge/src/mc116/java).
$(MOD_JAR_FORGE_MC116): $(shell git ls-files forge src/main/java .env.version) | ci-image
	@echo "[build] Building mc116 Forge jar (MC 1.16.5 compile target, Forge 36.2.x, SRG-renamed)..."
	@$(call in_ci_image_gradle,gradle -p forge build -PforgeTarget=mc116 --no-daemon --quiet)

.PHONY: build-forge-mc116
build-forge-mc116: $(MOD_JAR_FORGE_MC116) ## build the mc116 Forge jar, MC 1.16.x, SRG-reobfuscated (host gradlew + ForgeGradle 7)

# neoforge/ is a standalone Gradle build (`gradle -p neoforge`), not a subproject
# -- ModDevGradle and Fabric Loom are not supported in one project, and this way
# the four invocations above stay exactly as they were. First build of a band
# runs ModDevGradle's NeoForm pipeline (decompile + recompile Minecraft, several
# minutes); it is cached in GRADLE_USER_HOME afterwards. Prerequisites are the
# NeoForge build's own inputs, not MOD_SOURCES: a Fabric-only source change must
# not trigger the NeoForm pipeline.
$(MOD_JAR_NEO): $(shell git ls-files neoforge src/main/java .env.version) | ci-image
	@echo "[build] Building NeoForge band jar (MC 1.20.2-26.2, anchor 20.4.251)..."
	@$(call in_ci_image_gradle,gradle -p neoforge build -PneoTarget=all --no-daemon --quiet)

.PHONY: build
build: $(MOD_JAR_121) $(MOD_JAR_1192) $(MOD_JAR_114) $(MOD_JAR_26) $(MOD_JAR_NEO) ## build all five jars: four Fabric/Quilt eras + the NeoForge band (dockerized)

# Per-jar aliases of `build`'s five file targets, so CI can build each jar in
# its own parallel job (.NOTPARALLEL only serializes one make process).
.PHONY: build-121 build-1192 build-114 build-26 build-neo
build-121: $(MOD_JAR_121) ## build only the 1.21.x jar (dockerized)
build-1192: $(MOD_JAR_1192) ## build only the 1.19-1.20.2 jar (dockerized)
build-114: $(MOD_JAR_114) ## build only the 1.14.x jar, MC 1.14-1.18 (dockerized)
build-26: $(MOD_JAR_26) ## build only the 26.x jar (dockerized)
build-neo: $(MOD_JAR_NEO) ## build only the NeoForge band jar, MC 1.20.2-26.2 (dockerized)

.PHONY: test
test: ci-image ## offline suite: routing contract + 41 unit tests on all four targets (dockerized)
	@echo "[test] Offline version->jar routing contract..."
	@scripts/test-jar-routing.sh
	@echo "[test] Unit tests (1.21.x target)..."
	@$(call in_ci_image_gradle,gradle test --no-daemon --quiet)
	@echo "[test] Unit tests (1.19-1.20.2 target)..."
	@$(call in_ci_image_gradle,gradle test -PmcTarget=1192 --no-daemon --quiet)
	@echo "[test] Unit tests (1.14-1.18 target)..."
	@$(call in_ci_image_gradle,gradle test -PmcTarget=114 --no-daemon --quiet)
	@echo "[test] Unit tests (26.x target)..."
	@$(call in_ci_image_gradle,gradle test -PmcTarget=26 --no-daemon --quiet)
	@echo "[test] All targets passed."

.PHONY: lint-java
lint-java: ci-image ## checkstyle + PMD over Java sources (dockerized)
	@echo "[lint-java] checkstyle..."
	@$(call in_ci_image_gradle,gradle checkstyleMain --no-daemon --quiet)
	@echo "[lint-java] PMD..."
	@$(call in_ci_image_gradle,gradle pmdMain --no-daemon --quiet)

.PHONY: ci-tools-test
ci-tools-test: ci-image ## go test -race for tools/ (dockerized; the e2e-grid contract check used by e2e.yml)
	@docker run --rm \
	  --user "$$(id -u):$$(id -g)" \
	  -v "$(PWD):/work" -w /work/tools \
	  -v "$(GIT_COMMON_DIR):$(GIT_COMMON_DIR):ro" \
	  -v "$(if $(CI_CACHE_DIR),$(CI_CACHE_DIR),/tmp/commandsspy-ci-cache):/ci-cache" \
	  -e GOPROXY \
	  $(CI_IMAGE) go test -race -count=1 -cover ./...

.PHONY: ci-gen-matrix
ci-gen-matrix: ci-image ## compute the e2e version-matrix GitHub Actions outputs (dockerized; e2e.yml only)
	@docker run --rm \
	  --user "$$(id -u):$$(id -g)" \
	  -v "$(PWD):/work" -w /work/tools \
	  -v "$(GIT_COMMON_DIR):$(GIT_COMMON_DIR):ro" \
	  -v "$(if $(CI_CACHE_DIR),$(CI_CACHE_DIR),/tmp/commandsspy-ci-cache):/ci-cache" \
	  -e GOPROXY -e REPO_ROOT=/work -e EVENT_NAME -e FORCE_BANDS \
	  $(if $(GITHUB_OUTPUT),-e GITHUB_OUTPUT -v "$(GITHUB_OUTPUT):$(GITHUB_OUTPUT)",) \
	  $(CI_IMAGE) go run . gen-matrix

# make ci = the one fast gate (excludes e2e and the Gradle build). See docs/ci.md.
GOLANGCI_LINT_VERSION := 2.12.2
GOVULNCHECK_VERSION := 1.7.0

SH_SOURCES := $(shell git ls-files '*.sh')

.PHONY: lint-sh
# Pinned shellcheck in-container; ci-host is best-effort.
lint-sh: ## bash -n + pinned shellcheck over every tracked *.sh
	@for f in $(SH_SOURCES); do echo "bash -n $$f"; bash -n "$$f" || exit 1; done
	@test -z "$(SH_SOURCES)" && exit 0; \
	 shellcheck -S style $(SH_SOURCES)

.PHONY: go-fmt go-fmt-check go-vet go-lint go-vuln go-build go-test
go-fmt: ## rewrite tools/ with gofmt
	cd tools && gofmt -w .

go-fmt-check: ## report-only: CI must be able to fail (fleet rule)
	@unformatted="$$(cd tools && gofmt -l .)"; \
	 if [ -n "$$unformatted" ]; then echo "gofmt needed on:" >&2; echo "$$unformatted" >&2; exit 1; fi

go-vet: ## go vet over tools/
	cd tools && go vet ./...

# Dockerfile.ci bakes golangci-lint/govulncheck binaries at these exact
# versions (GOBIN=/usr/local/bin, never network-fetched at run time); ci-host
# also runs bare on a host with no baked binary, so both targets check the
# installed binary's version actually matches before trusting it, and fall
# back to the always-correct `go run pkg@version` otherwise. A version bump
# here without touching Dockerfile.ci's matching ARG degrades to the slow
# path instead of silently running the wrong version.
go-lint: ## golangci-lint (pinned) over tools/
	@if command -v golangci-lint > /dev/null 2>&1 && golangci-lint version 2>/dev/null | grep -q "$(GOLANGCI_LINT_VERSION)"; then \
	  cd tools && golangci-lint run; \
	else \
	  cd tools && go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v$(GOLANGCI_LINT_VERSION) run; \
	fi

go-vuln: ## govulncheck (pinned) over tools/
	@if command -v govulncheck > /dev/null 2>&1 && govulncheck -version 2>/dev/null | grep -q "$(GOVULNCHECK_VERSION)"; then \
	  cd tools && govulncheck ./...; \
	else \
	  cd tools && go run golang.org/x/vuln/cmd/govulncheck@v$(GOVULNCHECK_VERSION) ./...; \
	fi

go-build: ## the static binary is the deliverable - prove it every build
	cd tools && CGO_ENABLED=0 go build -trimpath -o /dev/null .

go-test: ## go test -race -count=1 -cover over tools/
	cd tools && go test -race -count=1 -cover ./...

.PHONY: ci-host
ci-host: lint-sh go-fmt-check go-vet go-lint go-build go-build-cross go-test go-vuln ## the raw quality gate on host tools (escape hatch; version skew possible)

# Both container architectures the e2e harness runs on, proven every gate run.
.PHONY: go-build-cross
go-build-cross: ## prove the static binary builds for linux+darwin
	cd tools && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o /dev/null .
	cd tools && CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags="-s -w" -o /dev/null .

# Tag is content-addressed: editing Dockerfile.ci changes the tag.
CI_IMAGE := ghcr.io/ashwalk33r/commandsspy-ci:$(shell git hash-object Dockerfile.ci | cut -c1-12)
# Disposable; CI_CACHE_DIR= for a cold run.
CI_CACHE_DIR ?= $(HOME)/.cache/commandsspy-ci-go
# Disposable Gradle dependency/toolchain cache; GRADLE_CACHE_DIR= for a cold run.
GRADLE_CACHE_DIR ?= $(HOME)/.cache/commandsspy-ci-gradle
# A linked worktree's .git is a pointer file to an absolute path under the
# main checkout's .git/worktrees/; `make -s ci-host`'s SH_SOURCES (git
# ls-files) is evaluated by the nested `make` that `ci` runs INSIDE the
# container, so it needs that path mounted too, or it fails silently under
# $(shell ...) and lint-sh silently checks zero files. Harmless to carry
# into the other container-run macros too — for a plain (non-worktree)
# checkout this just resolves to the repo's own already-mounted .git.
GIT_COMMON_DIR := $(shell cd "$$(git rev-parse --git-common-dir)" && pwd)

.PHONY: ci-image
ci-image: ## pull the pinned image from GHCR, or build (+push from CI) if not published yet
	@docker info > /dev/null 2>&1 || { \
	  echo "[ci] Docker not running or not reachable; fallback: make ci-host"; \
	  exit 1; \
	}
	@docker image inspect $(CI_IMAGE) > /dev/null 2>&1 || docker pull $(CI_IMAGE) > /dev/null 2>&1 || { \
	  echo "[ci] $(CI_IMAGE) not found locally or on GHCR; building..."; \
	  docker build -f Dockerfile.ci -t $(CI_IMAGE) . ; \
	  if [ -n "$${CI:-}" ]; then \
	    echo "[ci] Publishing $(CI_IMAGE) to GHCR..."; \
	    docker push $(CI_IMAGE) || echo "[ci] Push failed (non-fatal, this job still has the image locally)"; \
	  fi ; \
	}
	@mkdir -p $(if $(CI_CACHE_DIR),$(CI_CACHE_DIR),/tmp/commandsspy-ci-cache) $(if $(GRADLE_CACHE_DIR),$(GRADLE_CACHE_DIR),/tmp/commandsspy-ci-gradle)

# $(call in_ci_image,<command>) — run <command> inside the pinned CI_IMAGE
# with the repo mounted at /work (the image's WORKDIR) as the host UID.
define in_ci_image
docker run --rm \
  --user "$$(id -u):$$(id -g)" \
  -v "$(PWD):/work" \
  -v "$(GIT_COMMON_DIR):$(GIT_COMMON_DIR):ro" \
  -v "$(if $(CI_CACHE_DIR),$(CI_CACHE_DIR),/tmp/commandsspy-ci-cache):/ci-cache" \
  -e GOPROXY \
  $(CI_IMAGE) $(1)
endef

# Same as in_ci_image, plus a persistent Gradle cache — the `gradle` binary
# itself is baked into CI_IMAGE (see Dockerfile.ci), but without this mount
# every containerized run would still re-fetch every Fabric/Mojang/mappings
# jar for the project's own dependencies.
# LANG: the image has no locale set, so javac processes ForgeGradle's
# mavenizer forks default to ASCII and die on the UTF-8 '§' literals in the
# decompiled pre-1.17 Minecraft/Forge sources (mc116 target, issue #30);
# harmless everywhere else.
define in_ci_image_gradle
docker run --rm \
  --user "$$(id -u):$$(id -g)" \
  -v "$(PWD):/work" \
  -v "$(GIT_COMMON_DIR):$(GIT_COMMON_DIR):ro" \
  -v "$(if $(GRADLE_CACHE_DIR),$(GRADLE_CACHE_DIR),/tmp/commandsspy-ci-gradle):/gradle-cache" \
  -e GRADLE_USER_HOME=/gradle-cache \
  -e LANG=C.UTF-8 \
  $(CI_IMAGE) $(1)
endef

.PHONY: ci
ci: ci-image ## THE quality gate: go/shell lint+vet+build+test, inside the pinned image (hook + CI run this)
	@$(call in_ci_image,make -s ci-host)

.PHONY: all
all: build test lint-java ci e2e-ci ## one-stop local gate: jars, unit tests, Java lint, ci (lint/vet/go-test), e2e-ci matrix

.PHONY: hooks
hooks: ## arm the tracked pre-commit hook
	git config core.hooksPath .githooks

# `Done (12.345s)!` is emitted by the server itself.
.PHONY: e2e-times
e2e-times: ## report each version's server-reported boot time from the last e2e logs
	@for log in $(E2E_LOG_DIR)/*.log; do \
	  case "$$log" in *docker-build-*) continue ;; esac; \
	  version=$$(basename "$$log" .log); \
	  done_line=$$(grep -oE 'Done \([0-9.]+s\)' "$$log" | tail -1 || true); \
	  printf '%-10s %s\n' "$$version" "$${done_line:-<no Done line>}"; \
	done
