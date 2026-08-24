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

## 6. Each BTA release bundles its own loader fork, and Java 17 is not optional

The server packages do not share a loader:

| BTA releases | fabric-loader fork | Mixin |
|---|---|---|
| 7.3, 7.3_01, 7.3_02, 7.3_03 | `0.15.6-bta.7` | 0.8.5 |
| 7.3_04 | `0.18.4-bta.10` | 0.8.7 |
| 8.0, 8.0.1 | `0.18.4-bta.11` | 0.8.7 |

Two consequences, both measured:

1. The jar's declared `fabricloader` range must floor at the **oldest** fork in the
   declared set. Declaring `>=0.18.4-bta.11` refused the mod outright on four of the
   seven releases:

   ```
   Mod 'Commands Spy' (commandsspy) 1.8.0+bta7.3-8.0.1 requires version
   0.18.4-bta.11 or later of mod 'Fabric Loader', but only the wrong version is
   present: 0.15.6-bta.7!
   ```

   The e2e leg pins the exact fork per version instead — a predicate cannot express
   "whichever fork this package happens to ship".

2. **The 0.15.6-era packages do not run on Java 21 at all**, and the failure has
   nothing to do with this mod: HalpLibe, which BTA's own package ships in `mods/`,
   fails to apply under Mixin 0.8.5 on a Java 21 runtime —

   ```
   Error loading class: java/lang/invoke/LambdaMetafactory
       (java.lang.IllegalArgumentException: Unsupported class file major version 65)
   MixinPreProcessorException: Attach error for halplibe.mixins.json:MinecraftServerMixin
   ```

   The same package on Java 17 boots clean with the mod loaded:

   ```
   [13:50:09] [main/INFO] (FabricLoader/GameProvider) Loading Minecraft 7.3 with Fabric Loader 0.15.6-bta.7
   	- commandsspy 1.8.0+bta7.3-8.0.1
   [13:50:09] [main/INFO] (CommandsSpy) Loading CommandsSpy by Ultra_MC.
   [13:50:26] [Server thread/INFO] (Minecraft) Done (16545424688ns)! For help, type "help" or "?"
   ```

   So Java 17 is the JVM the band boots, not merely the version it declares. Note the
   log decoration differs between eras too — `[main/INFO] (CommandsSpy)` on 7.3,
   `[CommandsSpy/INFO]:` on 8.0.1 — so assertions match the message text, not the
   logger prefix.

## 7. The declared version strings are not the release names

fabric-loader **normalizes** the game version before matching a dependency
predicate. BTA's underscore releases are named `7.3_01` … `7.3_04`, and the boot
banner prints exactly that — but what `depends.minecraft` is compared against is
the semver form:

| BTA release | banner prints | predicate sees | e2e token |
|---|---|---|---|
| 7.3 | `7.3` | `7.3` | `bta7.3` |
| 7.3_01 … 7.3_04 | `7.3_0N` | `7.3.N` | `bta7.3_0N` |
| 8.0, 8.0.1 | `8.0`, `8.0.1` | `8.0`, `8.0.1` | `bta8.0`, `bta8.0.1` |

Declaring the underscore spelling refused the mod on four of the seven declared
releases, and the message is the one to recognise:

```
requires version 7.3, version 7.3_02, version 7.3_04, version 7.3_03, version 8.0,
version 7.3_01 or version 8.0.1 of 'Minecraft' (minecraft),
but only the wrong version is present: 7.3.4!
```

The e2e version tokens keep the underscore form, because they name the release
asset to download; `tools/gen_matrix_test.go` derives one list from the other so
the two spellings cannot drift apart.

## 8. No RCON

The `server.properties` BTA writes at boot carries no `rcon.*` key. Same
proven-absence shape as the Babric row, asserted rather than skipped.

## 9. The player-typed leg needed a new bot

`tools/beta.go`'s protocol 14 client fails immediately:

```
bot: beta (protocol 14): login phase, e2e_player1: handshake reply: got packet 0xfa, want 0x02
```

BTA's wire protocol keeps Beta's framing (bare id byte, big-endian fields, no
length prefix, no compression, no transport encryption) and changes everything
else:

- strings are `int16` **byte** count + UTF-8, not Beta's UTF-16BE code units
- the chat packet has THREE shapes across the declared line: `7.3` writes
  `type, UTF-8 string, encrypted`; `7.3_01`-`7.3_04` keep that order but switch the
  string to protocol 14's UTF-16BE form; `8.0`+ renames the packet and writes
  `type, encrypted, UTF-8 string`. The login tail moved too — `dimensionId` and
  `worldTypeId` are bytes before 8.0 and int32s from 8.0 on
- the protocol version is a per-release constant, not one number: `7.3`=29472,
  `7.3_01`=29441, `7.3_02`=29442, `7.3_03`=29443, `7.3_04`=29444, `8.0`=32768,
  `8.0.1`=32769. Each is the literal `PacketHandlerLogin` compares against, read
  out with `javap`; a mismatch is kicked as "Outdated client!"
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
