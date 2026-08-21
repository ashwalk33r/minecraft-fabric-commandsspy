# Upstream report: quilt-loader's pre-1.18 entrypoint gap

This file is a **bug report for quilt-loader, written to be pasted into
QuiltMC's tracker by a human**. It has not been filed. Nothing has been sent
to QuiltMC, to any maintainer, or to any forum, Discord or tracker.

It lives here because the defect is upstream and not ours to fix, but the
evidence for it is ours and is expensive to reassemble: it comes out of this
repo's e2e matrix, which boots real dedicated servers across nine Minecraft
versions on two loaders. Writing it down once means whoever decides to file
does not have to re-derive it.

**Before pasting this anywhere, work through
[the pre-filing checklist](#pre-filing-checklist-for-whoever-files-this) at
the end.** It is short, and every item on it is something that goes stale.

Everything from the next heading to the end of the file is the report. Select
from there and paste; nothing above it is meant to travel.

For this repo's own view of the same defect — what it costs us and how CI
holds it — see `docs/version-matrix.md` → "Quilt: the pre-1.18 entrypoint gap", and its
two follow-on subsections, "The gap is specific to the `main` call site" and
"Why a Quilt-native entrypoint cannot close this gap". That link is one-way:
this file quotes the version matrix, the version matrix does not yet point
back here.

---

## quilt-loader silently never invokes a mod's `main` entrypoint on dedicated servers below Minecraft 1.18

### Summary

On a dedicated server running Minecraft **1.17.1 or older**, quilt-loader
never calls the class declared in `quilt_loader.entrypoints.main`. There is no
crash, no exception, no warning, and no log line — the entrypoint simply never
runs. On Minecraft 1.18.2 and every version above it, the same jar on the same
loader on the same JVM works.

The mod is otherwise loaded, and the loader's entrypoint machinery is alive on
those versions. In a **single boot** on 1.16.5, the `preLaunch` entrypoint
declared in that same `quilt.mod.json` fires normally while `main` never does.
The mixin named in that same file is also applied. So the loader read the
metadata and acted on most of it — one stage, `main`, is silently skipped.

### Versions

| Component | Version |
| --- | --- |
| quilt-loader | `0.30.0` |
| quilt-installer | `0.15.1` |

Both are pinned in this project's e2e harness at
`scripts/e2e-run-one.sh:159-160`:

```sh
QUILT_LOADER_VERSION="${QUILT_LOADER_VERSION:-0.30.0}"
QUILT_INSTALLER_VERSION="${QUILT_INSTALLER_VERSION:-0.15.1}"
```

Servers are installed with
`java -jar quilt-installer.jar install server <mcversion> <loaderversion> --download-server`.

### The boundary

Measured across this project's e2e matrix. Every row is a real dedicated
server, booted in a container, with the mod jar in `mods/`, driven over the
console and RCON.

**`main` entrypoint never invoked — four versions:**

| Minecraft | Java |
| --- | --- |
| 1.14.4 | 8 |
| 1.15.2 | 8 |
| 1.16.5 | 8 |
| 1.17.1 | 17 |

**`main` entrypoint invoked normally — five versions:**

| Minecraft | Java |
| --- | --- |
| 1.18.2 | 17 |
| 1.19.2 | 17 |
| 1.19.4 | 17 |
| 1.20.2 | 17 |
| 1.21.11 | 25 |

The break is between **1.17.1 and 1.18.2**.

A note on the count, because it was undercounted here for a while: the prose
named three failing versions and omitted 1.15.2, while the code never did.
`scripts/e2e-entrypoint.sh` gates the expected-absent assertion on four
Minecraft lines —

```sh
if [ "$LOADER" = "quilt" ]; then
  case "$MC_VERSION" in
    1.14|1.14.*|1.15|1.15.*|1.16|1.16.*|1.17|1.17.*) QUILT_ENTRYPOINT_GAP=1 ;;
  esac
fi
```

— and `tools/gen_matrix.go` puts 1.15.2 in the band that actually runs:

```go
mc114 := band("mc114", "1.14.4", "1.15.2", "1.16.5", "1.17.1", "1.18.2")
```

1.15.2 therefore runs on Quilt on every pull request, under the
banner-must-be-absent assertion, and passes. The prose was stale, not the
measurement. **Four failing versions, not three.**

### The held-constant control

The strongest single run is the forward-JVM coverage row, which puts versions
from **both sides of the boundary on one JVM**: same Java 21, same mod jar
(one jar spans Minecraft 1.14–1.18.2 here), same loader, same container image,
same harness, same assertions. Only the Minecraft version differs.

On a pull request that row is **two versions — 1.14.4 and 1.18.2** — the two
ends, with the boundary between them. Reproduce with:

```console
$ cd tools && REPO_ROOT=.. EVENT_NAME=pull_request GOTOOLCHAIN=local go run . gen-matrix
mc114_java21:      2  ["1.14.4","1.18.2"]
```

That two-version run is the standing evidence: one JVM, one jar, one loader,
1.14.4 fails and 1.18.2 works.

The full five-version single-JVM sweep exists but is one manual dispatch away,
not part of routine CI — the row is emitted through a coverage helper
(`emitCoverage` in `tools/gen_matrix.go`) that trims to the band's endpoints on pull
requests:

```console
$ cd tools && REPO_ROOT=.. EVENT_NAME=workflow_dispatch GOTOOLCHAIN=local go run . gen-matrix
mc114_java21:      5  ["1.14.4","1.15.2","1.16.5","1.17.1","1.18.2"]
```

So: **two versions on one JVM is what has been run and re-run; five on one JVM
is available on demand.** Stated this way deliberately — a claim that five
versions ran on one JVM in a pull-request log would be refutable by opening
the log.

### What the loader does and does not do

This is the part that narrows it, and it is positive evidence rather than an
absence.

The jar's `quilt.mod.json` declares two things about the mod's behaviour: an
entrypoint and a mixin config.

```json
{
  "schema_version": 1,
  "quilt_loader": {
    "id": "commandsspy",
    "entrypoints": { "main": ["pl.m2x.commandsspy.CommandsSpyFabric"] }
  },
  "mixin": "commandsspy.mixins.json"
}
```

On the failing versions, **the mixin fires.** The mod's whole user-facing
function — intercepting dispatched commands and logging them — runs correctly
on 1.14.4, 1.15.2, 1.16.5 and 1.17.1. Issue `list` on the server console and
the command is logged, exactly as on 1.18.2.

That means quilt-loader parsed this jar's metadata, built a mod from it,
resolved its dependencies, and honoured the mixin config that metadata names.
The mod is in the loader's mod list; it is not being silently rejected at
discovery. The class named in `entrypoints.main`, in that same file the loader
just acted on, is never constructed and never called.

**The defect therefore sits between mod-list construction and entrypoint
invocation**, not in discovery, not in resolution, and not in mixin
application.

#### `preLaunch` fires in the same boot in which `main` does not

This is the sharpest evidence in the report, because it is an **intra-boot**
contrast: nothing about the environment, the jar, the loader build, the Java
floor or the Minecraft version can be confounding it. Both observations come
from one server start.

quilt-loader 0.30.0, Minecraft 1.16.5, Java 8, dedicated server:

```
[10:13:20] [main/INFO]: Loading 5 mods:
[10:13:23] [main/INFO]: CommandsSpy preLaunch: config loaded.
[10:13:48] [Server thread/INFO]: Done (17.977s)! For help, type "help"
[10:13:49] [Server thread/INFO]: [CommandsSpy] [Server] list
```

```console
$ grep 'Loading CommandsSpy by' 1.16.5-quilt.log
$
```

`quilt_loader.entrypoints.preLaunch` fires, 25 seconds before `Done (`.
`quilt_loader.entrypoints.main` never fires. Same boot, same jar, same
loader, same `quilt.mod.json`. The Fabric Loader control on 1.16.5, from the
identical commit, prints **both** lines.

What this establishes:

- **The loader's entrypoint dispatch machinery works below 1.18.** It reaches
  `QuiltLoaderImpl.invokePreLaunch`, walks `EntrypointUtils.invoke` →
  `EntrypointStorage` → `DefaultLanguageAdapter.create`, constructs a mod's
  entrypoint object and invokes it. The defect is *not* "quilt-loader does not
  dispatch entrypoints on old versions".
- **The failure is specific to the `main` stage**, which narrows it to that
  stage's registration/lookup path — not to discovery (the mixin already ruled
  that out), not to resolution, and not to dispatch in general.

One limit, stated so nobody over-reads it: `preLaunch` and `main` are
**different call sites**. Knot runs `preLaunch` before the game's main class is
loaded; `main` is reached through the `EntrypointPatch`-injected
`Hooks.startServer` path. A working `preLaunch` therefore does *not* prove the
`EntrypointPatch` injection fired. It proves the dispatch machinery downstream
of both is sound.

#### A relevant asymmetry in `invokePreLaunch`

From the pinned 0.30.0 binary (`javap -c -p
org/quiltmc/loader/impl/QuiltLoaderImpl.class`), `invokePreLaunch` dispatches
**two** stages in sequence: `pre_launch` against Quilt's own
`org.quiltmc.loader.api.entrypoint.PreLaunchEntrypoint`, then `preLaunch`
against Fabric's `net.fabricmc.loader.api.entrypoint.PreLaunchEntrypoint`.

The `main`, `client` and `server` stages all require the **Fabric** types
(`net.fabricmc.api.ModInitializer` and friends). So quilt-loader's `main` stage
is itself a Fabric-compatibility path. That seems the most promising place to
start looking.

### The metadata detail you will ask about first

The jar ships **both** `fabric.mod.json` and `quilt.mod.json`. Both name the
same entrypoint class. That class implements **Fabric's**
`net.fabricmc.api.ModInitializer`, not Quilt's `ModInitializer`:

```java
import net.fabricmc.api.ModInitializer;

public class CommandsSpyFabric implements ModInitializer {
    @Override
    public void onInitialize() { /* ... */ }
}
```

Two things follow, and the second is the interesting one:

1. Because the jar ships a `quilt.mod.json`, quilt-loader reads that file and
   does **not** fall back to `fabric.mod.json` for this jar. So the
   declaration being ignored is `quilt_loader.entrypoints.main` in
   `quilt.mod.json` — the Fabric metadata is not what is in play here.

   This is verified against the pinned loader binary, not inferred.
   Disassembling quilt-loader 0.30.0 from `maven.quiltmc.org`:

   ```sh
   javap -c -p org/quiltmc/loader/impl/plugin/QuiltPluginManagerImpl.class
   ```

   `QuiltPluginManagerImpl.plugins` is a `LinkedHashMap`; `runInternal`
   registers `StandardQuiltPlugin` before `StandardFabricPlugin`; and
   `scanZip` exits the plugin loop as soon as the Quilt plugin returns a
   non-empty result. `StandardFabricPlugin` is therefore never consulted for
   a jar carrying `quilt.mod.json`.

   It is also what makes the mixin evidence above decisive rather than
   ambiguous: the mixin config that fired is the one named in
   its top-level `"mixin"` key, and by the precedence above,
   `quilt.mod.json` is the only file the loader read for this jar.
2. The entrypoint class implements the Fabric interface while being declared
   in Quilt metadata. That is a supported combination on 1.18.2+ — it works
   there, on this exact jar — but **the adapter path that bridges a
   Fabric-typed entrypoint interface from a Quilt entrypoint declaration is
   the first place we would look**, since it is the one part of the pipeline
   that is doing something non-trivial and version-sensitive.

We have not confirmed suspicion (2). It is offered as the shortest path for
someone who knows the codebase, not as a finding.

### An earlier lead, and what was *not* done to check it

Labelled explicitly as **analysis, not measurement**, because the distinction
matters for how much weight to put on it.

By reading source across releases, quilt-loader's `EntrypointPatch` —
the bytecode patch that injects the entrypoint call into Minecraft's own main
class — appears **unchanged on the `EnvType.SERVER` path from 0.23.0 through
0.30.1-beta.2**, and identical to Fabric Loader 0.19.2's, which works on these
same Minecraft versions. Every in-patch failure mode there throws loudly, so
on that reading the hook is injected and reached, and the defect lies further
into the mod-loading pipeline. That reasoning is what produced the
"between mod-list construction and entrypoint invocation" claim above, and the
mixin evidence independently supports the same conclusion.

What was **not** done, stated plainly so nobody credits this with more rigour
than it has:

- No debugger was attached to a failing server.
- No loader-side debug logging was enabled.
- No decompiler output, diff, or artifact was retained — the comparison is not
  reproducible from anything in this repository. Searching this repo for
  `EntrypointPatch` returns prose and nothing else: no script, no captured
  output, no checked-in artifact.

Treat it as a **lead**. The measurements in the boundary, mixin and intra-boot
sections above stand on their own without it. Note also that the working
`preLaunch` does not corroborate it: `preLaunch` does not run through
`EntrypointPatch` at all, so whether the injected `Hooks.startServer` hook is
reached on these versions remains unmeasured.

### Reproducing without this project

Two recipes. The first needs no build toolchain at all.

#### A. Zero-build, using an already-published jar

The jar is public and already carries a `quilt.mod.json`.

1. Download `commandsspy-1.6.0+mc1.14.x.jar` from
   <https://modrinth.com/mod/commandsspy/versions> (the file tagged Quilt,
   covering Minecraft 1.14–1.18.2). Version 1.6.0 specifically — it declares
   only a `main` entrypoint, which keeps this recipe's expected output simple.
   Builds after it also declare a `preLaunch` entrypoint and will additionally
   print `CommandsSpy preLaunch: config loaded.` at boot; on the failing
   versions that line appears *while the banner still does not*, which is the
   intra-boot contrast above, reproducible in one run.
2. Install a Quilt **dedicated server** for Minecraft **1.16.5**:
   ```sh
   java -jar quilt-installer-0.15.1.jar install server 1.16.5 0.30.0 --download-server
   ```
   (Note: quilt-installer needs a Java 17+ JVM to *run*; the 1.16.5 server
   itself must then be launched on Java 8.)
3. Drop the jar into `mods/`, accept the EULA, and start the server.
4. Grep the log for the mod's startup banner:
   ```sh
   grep 'Loading CommandsSpy' logs/latest.log
   ```
   **Expected on 1.16.5: no match.** The entrypoint never ran.
5. Now, at the server console, type `list` and press enter. Look at the log
   again:
   ```sh
   grep 'CommandsSpy' logs/latest.log
   ```
   **Expected: the `list` command IS logged.** The mixin ran even though the
   entrypoint did not — this is the disambiguating step, not an afterthought.
6. Repeat steps 2–5 with Minecraft **1.18.2** (Java 17). Both greps now match:
   the banner appears at boot *and* `list` is logged.

Same jar, same loader, same installer. Only the Minecraft version changed.

#### B. From scratch, two files

If you would rather not trust someone else's jar, the minimum reproducer is
two files in one jar.

`quilt.mod.json`:

```json
{
  "schema_version": 1,
  "quilt_loader": {
    "group": "com.example",
    "id": "entrypointprobe",
    "version": "1.0.0",
    "intermediate_mappings": "net.fabricmc:intermediary",
    "entrypoints": { "main": ["com.example.Probe"] },
    "depends": [
      { "id": "quilt_loader", "versions": ">=0.30.0" }
    ],
    "metadata": { "name": "Entrypoint Probe" }
  }
}
```

`com/example/Probe.java` — use **Fabric's** `ModInitializer`, which is the
combination we actually measured (Quilt metadata, Fabric-typed entrypoint
interface):

```java
package com.example;

import net.fabricmc.api.ModInitializer;

public class Probe implements ModInitializer {
    @Override
    public void onInitialize() {
        System.out.println("ENTRYPOINT-PROBE: main entrypoint ran");
    }
}
```

Build it into a jar, drop it in `mods/` on a 1.16.5 Quilt dedicated server,
and grep for `ENTRYPOINT-PROBE`. Then do the same on 1.18.2.

Expected: silent on 1.16.5, prints on 1.18.2.

Do **not** bother swapping in a Quilt-typed initializer to compare: the `main`,
`client` and `server` stages all require the Fabric types, so a Quilt-typed one
is rejected on every Minecraft version, including the ones where `main` works.

To reproduce the intra-boot contrast in the same run, add a second entrypoint
to the same `quilt.mod.json`:

```json
"entrypoints": {
  "main":      ["com.example.Probe"],
  "preLaunch": ["com.example.PreProbe"]
}
```

with `PreProbe implements net.fabricmc.loader.api.entrypoint.PreLaunchEntrypoint`
printing its own distinct string. On 1.16.5 the preLaunch string prints and the
main string does not.

One hazard, measured: declaring a **Fabric**-typed class under the
`pre_launch` stage (underscore) hard-crashes the server at boot with
`LanguageAdapterException: ... cannot be cast to
org.quiltmc.loader.api.entrypoint.PreLaunchEntrypoint`. That is correct
behaviour — `pre_launch` is dispatched against Quilt's interface — and is why
the declaration above uses `preLaunch`, not `pre_launch`.

### What we are asking for

1. **Why does the `main` stage fail to register or invoke below Minecraft
   1.18, while `preLaunch` succeeds in the same boot?** That is the whole
   question, and it should be answerable from the code without reproducing
   anything: the mod is in the mod list (its mixin was applied from
   `quilt.mod.json`), the dispatch machinery runs (it invoked `preLaunch` from
   the same file), and only `main` is skipped, silently.
2. **Confirm or deny.** Is this a defect, or is pre-1.18 simply not supported?
3. **If pre-1.18 is out of scope, please publish that.** quilt-loader ships no
   minimum-supported-Minecraft statement anywhere we could find. A one-line
   floor in the README or on the download page would have saved this entire
   investigation, and would save it for the next person.

Either answer closes this for us. We are not asking for a fix.

### Non-goals

To be clear about what is and is not at stake:

- **Nothing user-visible is broken for us.** The mod's entire function —
  console, RCON and player command logging — works on all four affected
  versions, because the mixin applies independently of entrypoint invocation.
- **What we lose is a startup banner.** Nothing else. We previously also lost
  boot-time creation of our config file — it is written by the static
  initializer of the class `main` would have touched, so it slipped to the
  first executed command — but shipping a `preLaunch` entrypoint pulls that
  class up at boot again on exactly the affected versions. The banner is the
  only remaining symptom.
- **We are not blocked**, we are not requesting a workaround, and we are not
  asking for a backport. We ship these versions today with full functional
  coverage in CI.

This is filed — if it is filed — because a silent no-op is worth knowing
about, not because it is hurting us.

### A note for whoever fixes this

If this is fixed in a quilt-loader newer than 0.30.0, our CI will not notice.
The harness pins the loader version (`scripts/e2e-run-one.sh:159`), and our
assertion is *inverted* on the affected versions — we assert the banner is
**absent**, and fail the build if it ever appears
(`quilt-entrypoint-gap-closed-update-docs` in `scripts/e2e-entrypoint.sh`).
That tripwire only fires against the pinned loader. **Bumping the pin is
therefore the moment to re-test this**, and the moment our CI will tell us the
gap has closed.

### Pre-filing checklist for whoever files this

Do not paste this report without walking these. Each one is something that was
true when this was written and may not be true now.

- [ ] **Re-search the tracker.** A search of
      <https://github.com/QuiltMC/quilt-loader/issues> for *EntrypointPatch*,
      *entrypoint*, *legacy*, *1.16*, *1.17* and *onInitialize* found nothing.
      That search carries **no date** and was not re-run before you read this.
      Search again, including closed issues and discussions.
- [ ] **Re-test against the newest quilt-loader**, not 0.30.0. Recipe A takes
      about ten minutes. If it now passes, do not file — instead bump
      `QUILT_LOADER_VERSION` in `scripts/e2e-run-one.sh`, let the tripwire
      fire, and update `docs/version-matrix.md`.
- [ ] **Check whether quilt-loader has since published a minimum-supported
      Minecraft version.** If it has, and 1.17.1 is below it, ask #2 is
      already answered and there may be nothing to file.
- [ ] **Substitute your own environment details** — your OS, your JVM vendor
      and build, your exact loader and installer versions. The numbers above
      are this project's CI, not yours, and a maintainer will ask.
- [ ] **Re-read the "what was not done" list** and keep it in. Do not upgrade
      the `EntrypointPatch` analysis into a claim of measurement while
      trimming the report for length.
- [ ] **Confirm the download link in Recipe A still resolves** to a
      Quilt-tagged `mc1.14.x` file.
