# Upstream report: MinecraftForge's modern bootstrap on JDK 24+

This file is a **bug report for MinecraftForge, written to be pasted into its
tracker by a human**. It has not been filed. Nothing has been sent to
MinecraftForge, to any maintainer, or to any forum, Discord or tracker.

It lives here because the defect is upstream and not ours to fix, but the
evidence for it is ours and is expensive to reassemble: it comes out of this
repo's e2e matrix, which boots real dedicated servers across four loaders.
Writing it down once means whoever decides to file does not have to re-derive
it.

**Before pasting this anywhere, work through
[the pre-filing checklist](#pre-filing-checklist-for-whoever-files-this) at
the end.** It is short, and every item on it is something that goes stale —
the first item in particular, because this defect will have hit every project
shipping `nimbus-jose-jwt` with a `module-info`, and a duplicate report costs
credibility.

Everything from the next heading to the end of the file is the report. Select
from there and paste; nothing above it is meant to travel.

For this repo's own view of the same defect — which versions it costs us and
how CI is arranged around it — see the wiki,
[Supported Versions -> "Forge `modern` is Java 21 only"](https://github.com/ashwalk33r/minecraft-fabric-commandsspy/wiki/Supported-Versions#forge-modern-is-java-21-only),
and issue #66.

---

## Forge's modern bootstrap cannot start on JDK 24 or newer: `jdk.crypto.ec` was removed

### Summary

`net.minecraftforge.bootstrap` fails module resolution before Minecraft starts,
on any JDK from 24 onward, because `com.nimbusds.jose.jwt` declares a
`requires` on `jdk.crypto.ec` — a platform module **removed in JDK 24**, when
SunEC was folded into `java.base`.

There is no user-side workaround. `--add-modules jdk.crypto.ec` cannot add a
module the JDK no longer ships.

### Reproduction

A vanilla Forge dedicated server for a `modern`-era Minecraft version, started
on JDK 25 or 26. No mods are required to reproduce — the failure is in the
bootstrap, before mod scanning.

```
Exception in thread "main" java.lang.reflect.InvocationTargetException
	at net.minecraftforge.bootstrap.shim.Main.main(Main.java:101)
Caused by: java.lang.reflect.InvocationTargetException
	at net.minecraftforge.bootstrap.Bootstrap.bootstrapMain(Bootstrap.java:133)
	at net.minecraftforge.bootstrap.ForgeBootstrap.main(ForgeBootstrap.java:19)
Caused by: java.lang.module.FindException: Module jdk.crypto.ec not found, required by com.nimbusds.jose.jwt
	at java.base/java.lang.module.Resolver.findFail(Unknown Source)
	at java.base/java.lang.module.Configuration.resolveAndBind(Unknown Source)
	at net.minecraftforge.bootstrap@2.1.7/net.minecraftforge.bootstrap.Bootstrap.moduleMain(Bootstrap.java:166)
```

Observed with `net.minecraftforge.bootstrap` **2.1.7**.

### What was measured, and what was not

| Minecraft / Forge era | JVM | Result |
|---|---|---|
| 1.21.5, `modern` | 21 | **boots** — green on every CI run |
| 1.21.5, `modern` | 25 | boot-failed, trace above |
| 1.21.5, `modern` | 26 | boot-failed, trace above (twice, in CI) |
| 26.2, EventBus-7 era | 26 | **boots** — same CI matrix, same JVM, same jar staging |

**Java 22, 23 and 24 were not measured** — this project's harness ships no
image for those releases. The JDK-24 boundary is cited from the release note
that removed `jdk.crypto.ec`, not bisected here. The honest claim is: **last
known-good 21; known-bad 25 and 26; 22-24 unmeasured.**

### The control that makes this a Forge-generation bug

The last row of that table is the important one. In a single CI run
([32499679004](https://github.com/ashwalk33r/minecraft-fabric-commandsspy/actions/runs/32499679004)),
the same job, on the same JVM, with the same jar-staging path, booted **26.2
successfully** and **failed on 1.21.5**:

```
success   e2e 1.21.5, 26.2 java 26 (forge, above floor) / 26.2
failure   e2e 1.21.5, 26.2 java 26 (forge, above floor) / 1.21.5
```

So this is not "a new JDK breaks old software" in general. A later Forge
generation's bootstrap does not have the problem; the modern-era one does.

### Scope

- **Affected:** Forge's modern-era bootstrap. In this project's terms that is
  Minecraft 1.20.6 through 1.21.5.
- **Unaffected:** the EventBus-7 era (1.21.6-26.2), the legacy and mc116 eras,
  NeoForge, and Fabric/Quilt. NeoForge specifically boots 1.21.1 on Java 25
  without trouble.

### Suggested fix

Drop or shade the `nimbus-jose-jwt` module requirement in the bootstrap's
module path, so resolution does not depend on a platform module that no longer
exists. The dependency's `module-info` is the proximate cause; the bootstrap is
where it becomes fatal.

### Impact

A server operator on a current JDK has no path forward on these Minecraft
versions other than installing an older JDK. Because the failure is in module
resolution, it happens before any Forge or mod logging, so the only diagnostic
is the stack trace above — there is nothing in the server log to read.

---

## Pre-filing checklist for whoever files this

1. **Search MinecraftForge's tracker first.** `jdk.crypto.ec` being removed in
   JDK 24 will have hit every project shipping `nimbus-jose-jwt` with a
   `module-info`. There may already be an issue, or a fix already in a later
   Forge line. A duplicate costs credibility.
2. **Re-run the reproduction against current builds** before claiming it is
   still live. This repo can regenerate a fresh log in one command:
   ```
   FORGE_MODERN_JAVA_CEILING=26 make e2e VERSIONS=1.21.5 LOADER=forge JAVA=25
   ```
   That override exists precisely so the band stays re-probeable after an
   upstream fix.
3. **Check the bootstrap version.** The trace above is from
   `net.minecraftforge.bootstrap` 2.1.7. If current builds ship a newer one,
   re-derive rather than quoting this.
4. **Do not claim the JDK-24 boundary was bisected.** It was not. Keep the
   "last known-good 21, known-bad 25 and 26, 22-24 unmeasured" framing, or
   measure the missing rungs first.
5. **Check whether CI run 32499679004 is still retrievable.** GitHub expires
   logs and artifacts; if it has gone, the control result needs regenerating
   before it is cited as evidence.
