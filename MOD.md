# CommandsSpy
A mod to log commands to the server console. Both existing and non-existing commands are logged — on every supported loader, verified on booted servers (Fabric, Quilt, Forge and NeoForge each log a command name their dispatcher cannot resolve). Commands that are permitted or not are added to the server logs as well.

## Installation
Simply place the mod into the mods directory of your server, and it will start monitoring executed commands.
On startup, the config file will be created automatically.
## Compatibility

The mod has no dependencies beyond your loader — Fabric API/QSL is NOT required,
and neither is any Forge/NeoForge library.

Which jar to install for your Minecraft version and loader, the Java version each
one needs, and which combinations are proven by a booted server in CI:

**[Supported Versions](https://github.com/ashwalk33r/minecraft-fabric-commandsspy/wiki/Supported-Versions)** (wiki)

That page is the single source of truth. It distinguishes what the jars *declare*
from what CI has actually *booted*, and every figure on it carries the command that
regenerates it.

Short version: Fabric and Quilt share one jar per era, so a Quilt tag on a release
listing is an accurate claim rather than a Fabric-compat assumption. Forge needs a
different jar per mapping era. NeoForge takes a single jar across its whole history. Babric takes a single jar for a single Minecraft version, Beta 1.7.3, and it
is the one platform where the mod hooks something other than a Brigadier
dispatcher — Beta 1.7.3 has no Brigadier, so the jar hooks the game's two
hand-rolled command seams instead. BTA — "Better than Adventure!" — takes its
own jar again: it is a fork of the Beta 1.7.3 *game*, shipped unobfuscated with
its own class layout and its own loader fork, so the Babric jar cannot load
there. That jar declares an enumerated list of BTA releases from 7.3 up, because
the Brigadier dispatcher it hooks does not exist in BTA 7.2 and older.

## Examples
Player "Ultra_MC" executing "/gamemode creative" command is logged without arguments in server logs when using config `"logArguments": false` (default):
```
[18:57:06] [Server thread/INFO]: [CommandsSpy] [Player: Ultra_MC] gamemode
```

Player "Ultra_MC" executing "/gamemode creative" command is logged with arguments in server logs when using config `"logArguments": true`:
```
[18:57:06] [Server thread/INFO]: [CommandsSpy] [Player: Ultra_MC] gamemode creative
```

Player "Ultra_MC" executing "/op" command is logged in server logs:
```
[18:56:38] [Server thread/INFO]: [CommandsSpy] [Player: Ultra_MC] op
```

Server console executing "/list" is logged in server logs:
```
[18:53:02] [Server thread/INFO]: [CommandsSpy] [Server] list
```

RCON executing "/save-all" is logged in server logs:
```
[09:25:01] [Server thread/INFO]: [CommandsSpy] [Rcon] save-all
```

## Configuration
### Config file `config/commands-spy.json`
#### Initial config
```
{
"blacklist": [],
"logArguments": false
}
```

#### blacklist
To prevent logging certain commands, add them to the blacklist.
Example - to maintain privacy of players' conversations, you can avoid logging commands `tell` and `t`.
```
"blacklist": ["tell", "t"]
```

#### log arguments
Command arguments are not logged by default: `"logArguments": false`.
Example: `/a b c` - `[CommandsSpy] [Player: Ultra_MC] a`

To log arguments of all commands, use `"logArguments": true`.
Example: `/a b c` - `[CommandsSpy] [Player: Ultra_MC] a b c`

## About the version lists on the download page

Each file's Minecraft version list is the range that file's metadata **declares** —
every Minecraft release the loader will accept it on. Since 1.7.1 those lists are
generated from the repository's own coverage table (`docs/modrinth-versions.tsv`),
and every version on them is booted as a real dedicated server by CI, or carries a
written reason why it is not. Nothing is listed that cannot be installed.

Four things a version list cannot say, so they are said here:

- **`+mc1.21.x-forge` is Java 21 only.** Java 25 and above crash before Minecraft
  starts — a Forge bootstrap limitation, not a mod one.
- **Quilt below 1.14.4 does not exist.** Quilt Loader publishes no build for
  Minecraft 1.14–1.14.3, so those four releases are on `+mc1.14.x-fabric` (tagged
  Fabric only) and not on `+mc1.14.x` (Fabric and Quilt). Same jar, two listings,
  because a listing cannot exclude one loader from one version.
- **On Quilt below 1.18.2 the startup banner is missing.** Quilt Loader never
  invokes the mod's `main` entrypoint there. It costs the `Loading CommandsSpy`
  line and nothing else: command logging, config creation and every other behaviour
  are proven on those versions.
- **Some NeoForge lines have only prerelease loader builds** (1.20.3, 1.20.5,
  1.21.2, 1.21.6, 1.21.7, 1.21.9, 26.1, 26.1.1). The mod installs there; CI does
  not boot them, because a green build should not depend on beta loader code.

The full matrix — which (Minecraft, loader, Java) combinations are proven, which are
declared, and the command that regenerates every figure — is in the wiki:
[Supported Versions](https://github.com/ashwalk33r/minecraft-fabric-commandsspy/wiki/Supported-Versions).

