#!/bin/bash
set -euo pipefail

MC_VERSION="${MC_VERSION:-1.21}"
BOOT_TIMEOUT="${BOOT_TIMEOUT:-180}"
INSTALLER_URL="https://maven.fabricmc.net/net/fabricmc/fabric-installer/1.0.1/fabric-installer-1.0.1.jar"

cd /fabric-server

# Determine Fabric Loader version based on Minecraft version
if [[ "$MC_VERSION" =~ ^1\.21 ]]; then
  LOADER_VERSION="0.16.5"
elif [[ "$MC_VERSION" =~ ^26 ]]; then
  LOADER_VERSION="0.19.3"
else
  echo "ERROR: unsupported Minecraft version $MC_VERSION"
  exit 1
fi

# Download and run Fabric installer
echo "[smoke] Installing Fabric for Minecraft $MC_VERSION with Loader $LOADER_VERSION"
java -jar <(curl -fsSL "$INSTALLER_URL") server \
  -dir . \
  -mcversion "$MC_VERSION" \
  -loader "$LOADER_VERSION" \
  -downloadMinecraft 2>&1 | tail -20

# Copy mod into place if it exists
if [ -f "/tmp/mod.jar" ]; then
  mkdir -p mods
  cp /tmp/mod.jar mods/
  echo "[smoke] Mod JAR copied to mods/"
fi

# Ensure eula.txt exists
echo 'eula=true' > eula.txt

# Boot server and wait for completion
echo "[smoke] Starting Fabric server for Minecraft $MC_VERSION..."
timeout "$BOOT_TIMEOUT" java -jar fabric-server-launch.jar nogui > server.log 2>&1 || rc=$?
rc=${rc:-0}

# Check for successful boot
if grep -q 'Done (' server.log; then
  echo "[smoke] ✓ Server booted successfully on Minecraft $MC_VERSION"
  exit 0
else
  echo "[smoke] ✗ Server failed to boot on Minecraft $MC_VERSION (rc=$rc)"
  echo "[smoke] Last 30 lines of server.log:"
  tail -30 server.log
  exit 1
fi
