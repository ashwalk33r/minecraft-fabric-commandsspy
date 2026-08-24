# BTA toolchain spike

Task 1 of the BTA loader support work ([#90](https://github.com/ashwalk33r/minecraft-fabric-commandsspy/issues/90)).
Nothing in this repository had ever built against BTA, so the work opens with a
spike whose deliverable is knowledge rather than code. This page records what was
run on 2026-08-24 and what it printed. Where a result contradicts an assumption,
the observed value wins.

Everything below was executed on this machine. Nothing here is read from
documentation.

## Summary

The whole chain works. A jar compiled against BTA 8.0.1 through BTA's Loom line
was installed into BTA's own modded server package, booted on Java 21, and logged
both a console command and a player-typed command — the second driven by a Go bot
speaking BTA's login protocol.

Four things worth not re-deriving: BTA is **not obfuscated** so Loom refuses a
`mappings` declaration at all; the manifest still lists Beta-era client libraries
that no longer resolve and must be excluded; there is **no `remapJar` task**, so
the shipped artifact is plain `jar`; and BTA's console source name is `Server`,
not Babric's `CONSOLE`.

## 1. The platform is a different game binary

`net/minecraft/server/network/ServerPlayNetworkHandler` and
`net/minecraft/server/command/ServerCommandHandler` — the two seams the Babric jar
mixes into — do not exist in any BTA jar. BTA ships unobfuscated classes under its
own layout.

## 2. One seam, Brigadier-backed, stable across a BTA major

```
net.minecraft.core.net.command.CommandManager#execute(String, CommandSource) -> int
net.minecraft.core.net.command.CommandSource#getName() -> String
net.minecraft.core.net.command.CommandSource#getSender() -> Player   // null for console
```

Both dispatch paths converge on `execute`, from `javap -c` on the shipped server
jars:

- player-typed — `PacketHandlerServer#handleMessage` tests `startsWith("/")`, calls
  `handleSlashCommand`, which calls
  `world.getCommandManager().execute(message.substring(1), new ServerCommandSource(server, player))`.
  **The leading slash is already stripped by the game.**
- console — `MinecraftServer`'s command queue drains into the same `execute` with a
  `ConsoleCommandSource`.

Present and identically shaped in 7.3 and 8.0.1. In 8.0.1 the player packet handler
method is `handleMessage(PacketMessage)`; in 7.3 it was `handleChat(PacketChat)` —
which is exactly why the mixin targets `CommandManager` and not the packet handler.

## 3. The seam's floor is BTA 7.3

`net/minecraft/core/net/command/CommandManager.class` is absent from the 7.1 and
7.2 server jars (they carry the pre-Brigadier `Command`/`CommandHandler` pair) and
present from 7.3 on. Regenerate:

```bash
unzip -l bta-<ver>-server.jar | grep -c 'core/net/command/CommandManager\.class'
```

## 4. Build toolchain — what actually resolves

BTA's current example mod (branch `8.0`) uses plain `net.fabricmc.fabric-loom`
`1.15-SNAPSHOT` from the Signalum maven, not `babric-loom`:

| Coordinate | Value | Where from |
|---|---|---|
| Loom plugin | `net.fabricmc.fabric-loom:1.15-SNAPSHOT` | `https://maven.thesignalumproject.net/infrastructure` |
| Loader fork | `net.fabricmc:fabric-loader:0.18.4-bta.11` | same maven (also a GitHub release asset on `Turnip-Labs/fabric-loader`) |
| Game jar | `minecraft "::8.0.1"` | resolved through `loom.customMinecraftMetadata` |
| Version metadata | `https://downloads.betterthanadventure.net/bta-client/release/v8.0.1/manifest.json` | BTA's own CDN; carries sha1-pinned `client` **and `server`** downloads |

Three build facts that cost a failed run each:

1. `mappings loom.layered() {}` fails with **"Cannot configure layered mappings in a
   non-obfuscated environment"**. Declare no mappings at all.
2. Dependency resolution dies on `org.lwjgl.lwjgl:lwjgl:2.9.4-babric.1` and
   `lwjgl_util`, which the manifest lists and no maven serves. Exclude the Beta-era
   client library groups, as BTA's example mod does.
3. With nothing to remap there is **no `remapJar` task**; `jar` is the shipped
   artifact, so the `-bta` classifier goes there.

## 5. The measured boot, with the mod installed

Server package: `bta_fabric_server_8.0.1.zip` from
`Turnip-Labs/bta-fabric-instance-repo` releases — `fabric-server-launch.jar` (thin,
`Class-Path` manifest) + `libraries/` + `server.jar` + `mods/` (ships
`halplibe-6.1.4+8.0.jar`) + `start.sh`. The mod jar was dropped into `mods/`.

```
[15:30:59] [FabricLoader/INFO]: Loading 6 mods:
	- commandsspy 1.8.0+bta7.3-8.0.1
	   \-- mixinextras 0.5.0
[15:30:59] [FabricLoader/Mixin/INFO]: Compatibility level set to JAVA_17
[15:31:00] [CommandsSpy/INFO]: Loading CommandsSpy by Ultra_MC.
[15:31:46] [CommandsSpy/INFO]: [CommandsSpy] [Server] help
[15:31:46] [CommandsSpy/INFO]: [CommandsSpy] [Server] notacommand
[15:31:56] [CommandsSpy/INFO]: [CommandsSpy] [Player: e2e_player1] me
```

Four consequences for the harness:

1. The loader reports the game as **`8.0.1`**, not `b1.7.3`: `Loading Minecraft
   8.0.1 with Fabric Loader 0.18.4-bta.11`. `fabric.mod.json`'s `depends.minecraft`
   is written against BTA's version line.
2. `Mappings not present!` — no intermediary namespace; mixins set `remap = false`.
3. The ready line is nanoseconds: `Done (13297414790ns)!`. A `Done \([\d.]+s\)!`
   matcher never fires.
4. The console source name is **`Server`**, like every modern loader and unlike
   Babric's `CONSOLE`, so no new `CONSOLE_SOURCE_NAME` case is needed.

Log format is a third variant: `[HH:MM:SS] [LogUtils/INFO]:` for the game and
`[HH:MM:SS] [CommandsSpy/INFO]:` for the mod.

## 6. No RCON

The `server.properties` BTA writes at boot carries no `rcon.*` key. Same
proven-absence shape as the Babric row, asserted rather than skipped.

## 7. The player-typed leg needed a new bot

`tools/beta.go`'s protocol 14 client fails immediately:

```
bot: beta (protocol 14): login phase, e2e_player1: handshake reply: got packet 0xfa, want 0x02
```

BTA's wire protocol keeps Beta's framing (bare id byte, big-endian fields, no
length prefix, no compression, no transport encryption) and changes everything
else:

- strings are `int16` **byte** count + UTF-8, not Beta's UTF-16BE code units
- protocol version is `32769`
- the server opens with an unsolicited `0xFA` custom payload (a HalpLibe artifact,
  not protocol — skip any `0xFA` rather than expect a channel)
- `0x01` Login carries protocol, username, a UUID, an **RSA public key string**,
  seed, dimension, world-type id and packet delay
- after login the server sends `0x88` with a per-player AES key, RSA-encrypted to
  the key the client supplied
- `0x03` Message carries a type byte and an `encrypted` boolean before the string.
  **Server→client chat is always AES-encrypted; client→server is not required to
  be** — send `encrypted = false` and the server takes the line verbatim

The command path is `handleMessage` → `startsWith("/")` → `handleSlashCommand` →
`CommandManager.execute`. Nothing must be sent to stay connected: no keep-alive
reply, no `PacketRequestCommandManager`.

A vanilla BTA server **never logs a command line itself**, only chat. The e2e
assertion therefore reads CommandsSpy's own output, which is what the log above
shows.
