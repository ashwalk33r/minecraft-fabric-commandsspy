.PHONY: smoke
SMOKE_VERSIONS := 1.21 1.21.11
SMOKE_LOG_DIR := build/smoke-logs
MOD_JAR := build/libs/commandsspy-1.2.1.jar

smoke: clean-smoke build $(MOD_JAR)
	@echo "[smoke] Starting smoke tests for Minecraft versions: $(SMOKE_VERSIONS)"
	@mkdir -p $(SMOKE_LOG_DIR)
	@failed=0; \
	for version in $(SMOKE_VERSIONS); do \
	  echo "[smoke] Testing Minecraft $$version..."; \
	  docker build --build-arg MC_VERSION=$$version \
	    -t commandsspy-smoke:$$version . > /dev/null 2>&1 || { \
	    echo "[smoke] ✗ Docker build failed for $$version"; \
	    failed=1; \
	    continue; \
	  }; \
	  mkdir -p $(SMOKE_LOG_DIR)/$$version; \
	  if docker run --rm \
	    -e MC_VERSION=$$version \
	    -e BOOT_TIMEOUT=180 \
	    -v $(PWD)/$(MOD_JAR):/tmp/mod.jar:ro \
	    commandsspy-smoke:$$version > $(SMOKE_LOG_DIR)/$$version/server.log 2>&1; then \
	    echo "[smoke] ✓ Passed: Minecraft $$version"; \
	  else \
	    echo "[smoke] ✗ Boot failed for Minecraft $$version (see logs at $(SMOKE_LOG_DIR)/$$version/server.log)"; \
	    failed=1; \
	  fi; \
	  docker rmi commandsspy-smoke:$$version > /dev/null 2>&1 || true; \
	done; \
	if [ $$failed -eq 1 ]; then \
	  echo "[smoke] Some versions failed. Logs retained in $(SMOKE_LOG_DIR)/"; \
	  exit 1; \
	fi
	@echo "[smoke] All versions passed!"
	@echo "[smoke] Logs saved to $(SMOKE_LOG_DIR)/"

.PHONY: clean-smoke
clean-smoke:
	@rm -rf $(SMOKE_LOG_DIR)
	@docker ps -a --filter "ancestor=commandsspy-smoke:*" -q 2>/dev/null | xargs -r docker rm -f 2>/dev/null || true
	@docker images -q "commandsspy-smoke:*" 2>/dev/null | xargs -r docker rmi -f 2>/dev/null || true

.PHONY: build
build:
	@./gradlew build --no-daemon --quiet
