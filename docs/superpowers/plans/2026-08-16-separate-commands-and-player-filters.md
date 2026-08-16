# Separate Commands and Player Filters Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix PR #14 which confuses `commandsBlacklist` (commands to skip logging) with `playersBlacklist`/`playersWhitelist` (players to include/exclude) by using clearly-named separate config fields.

**Architecture:** Rename config field `blacklist` → `commandsBlacklist` (with backward-compat migration), add `playersBlacklist` and `playersWhitelist` fields. Add `CommandsSpyPlayerFilter` helper, wire it into `CommandManagerMixin`. The `CommandsSpyBlacklist` class is fed from `commandsBlacklist` only.

**Tech Stack:** Java 17, Fabric Mod (Minecraft 1.21.x), Gradle, JUnit Jupiter 5.10.2

## Global Constraints

- Package: `pl.m2x.commandsspy`
- Config file: `config/commands-spy.json`
- Backward compat: old `blacklist` JSON key must still be read and mapped to `commandsBlacklist`
- Case-insensitive matching for both command and player names
- No new dependencies beyond JUnit Jupiter (already added by PR #14 commits)

---

### Task 1: Rename config field and add player filter fields with backward compat

**Files:**
- Modify: `src/main/java/pl/m2x/commandsspy/CommandsSpyConfig.java`

**Interfaces:**
- Produces: `public List<String> commandsBlacklist`, `public List<String> playersBlacklist`, `public List<String> playersWhitelist` fields on `CommandsSpyConfig`

- [ ] **Step 1: Update CommandsSpyConfig**

Replace the existing `blacklist` field with three clearly named fields. Add a `@SerializedName` or custom deserialization to preserve backward compat with old `blacklist` key. Simplest approach: keep `blacklist` as a deprecated alias that is read into `commandsBlacklist` via a custom `load()` migration step.

```java
package pl.m2x.commandsspy;

import com.google.gson.Gson;
import com.google.gson.GsonBuilder;
import com.google.gson.JsonObject;
import com.google.gson.JsonParser;
import net.fabricmc.loader.api.FabricLoader;

import java.io.*;
import java.nio.file.Files;
import java.nio.file.Path;
import java.util.ArrayList;
import java.util.List;

public class CommandsSpyConfig {
    private static final String CONFIG_FILE = "commands-spy.json";
    private static final Gson GSON = new GsonBuilder().setPrettyPrinting().create();

    public List<String> commandsBlacklist = new ArrayList<>();
    public List<String> playersBlacklist = new ArrayList<>();
    public List<String> playersWhitelist = new ArrayList<>();
    public boolean logArguments = false;

    public static CommandsSpyConfig load() {
        Path configPath = FabricLoader.getInstance().getConfigDir().resolve(CONFIG_FILE);
        CommandsSpyConfig config;

        if (Files.exists(configPath)) {
            try (Reader reader = Files.newBufferedReader(configPath)) {
                JsonObject json = JsonParser.parseReader(reader).getAsJsonObject();
                // ponytail: backward-compat migration — remove when old configs are gone
                if (json.has("blacklist") && !json.has("commandsBlacklist")) {
                    json.add("commandsBlacklist", json.get("blacklist"));
                }
                config = GSON.fromJson(json, CommandsSpyConfig.class);
            } catch (IOException e) {
                throw new RuntimeException("Error reading config file", e);
            }
        } else {
            config = new CommandsSpyConfig();
            config.save();
        }

        return config;
    }

    public void save() {
        Path configPath = FabricLoader.getInstance().getConfigDir().resolve(CONFIG_FILE);
        try (Writer writer = Files.newBufferedWriter(configPath)) {
            GSON.toJson(this, writer);
        } catch (IOException e) {
            throw new RuntimeException("Error writing config file", e);
        }
    }
}
```

- [ ] **Step 2: Commit**

```bash
git add src/main/java/pl/m2x/commandsspy/CommandsSpyConfig.java
git commit -m "refactor: rename blacklist->commandsBlacklist, add playersBlacklist+playersWhitelist"
```

---

### Task 2: Update CommandsSpy to use commandsBlacklist

**Files:**
- Modify: `src/main/java/pl/m2x/commandsspy/CommandsSpy.java`

**Interfaces:**
- Consumes: `CommandsSpyConfig.commandsBlacklist`
- Produces: `public static final CommandsSpyBlacklist BLACKLIST` fed from `commandsBlacklist`

- [ ] **Step 1: Update CommandsSpy.java**

```java
package pl.m2x.commandsspy;

import net.fabricmc.api.ModInitializer;
import org.apache.logging.log4j.LogManager;
import org.apache.logging.log4j.core.Logger;

public class CommandsSpy implements ModInitializer {
    public static final Logger LOGGER = (Logger) LogManager.getLogger("CommandsSpy");
    public static final CommandsSpyConfig CONFIG = CommandsSpyConfig.load();
    public static final CommandsSpyBlacklist BLACKLIST = new CommandsSpyBlacklist(CONFIG.commandsBlacklist);

    @Override
    public void onInitialize() {
        LOGGER.info("Loading CommandsSpy by Ultra_MC.");
    }

    public static void logCommand(String command, String source) {
        LOGGER.info("[CommandsSpy] [{}] {}", source, command);
    }
}
```

- [ ] **Step 2: Commit**

```bash
git add src/main/java/pl/m2x/commandsspy/CommandsSpy.java
git commit -m "fix: feed BLACKLIST from commandsBlacklist, not the old blacklist field"
```

---

### Task 3: Add CommandsSpyPlayerFilter

**Files:**
- Create: `src/main/java/pl/m2x/commandsspy/CommandsSpyPlayerFilter.java`
- Create: `src/test/java/pl/m2x/commandsspy/CommandsSpyPlayerFilterTest.java`

**Interfaces:**
- Produces: `public static boolean shouldLogPlayer(String playerName, List<String> playersBlacklist, List<String> playersWhitelist)`

- [ ] **Step 1: Add JUnit to build.gradle**

In `build.gradle`, inside the `dependencies {}` block add:
```groovy
testImplementation "org.junit.jupiter:junit-jupiter:5.10.2"
```
And add after the `dependencies {}` block:
```groovy
test {
    useJUnitPlatform()
}
```

- [ ] **Step 2: Write the failing test**

Create `src/test/java/pl/m2x/commandsspy/CommandsSpyPlayerFilterTest.java`:
```java
package pl.m2x.commandsspy;

import org.junit.jupiter.api.Test;
import java.util.List;
import static org.junit.jupiter.api.Assertions.*;

class CommandsSpyPlayerFilterTest {
    private static final String PLAYER = "Ultra_MC";

    @Test
    void allowsPlayerWhenWhitelistEmptyAndNotBlacklisted() {
        assertTrue(CommandsSpyPlayerFilter.shouldLogPlayer(PLAYER, List.of("Other"), List.of()),
                "Player should be logged when whitelist is empty and player is not blacklisted");
    }

    @Test
    void deniesPlayerWhenBlacklistedEvenIfWhitelisted() {
        assertFalse(CommandsSpyPlayerFilter.shouldLogPlayer(PLAYER, List.of("ultra_mc"), List.of(PLAYER)),
                "Blacklist must override whitelist");
    }

    @Test
    void deniesPlayerWhenWhitelistNonEmptyAndPlayerMissing() {
        assertFalse(CommandsSpyPlayerFilter.shouldLogPlayer(PLAYER, List.of(), List.of("SomeoneElse")),
                "Player should be denied when whitelist is non-empty and does not include the player");
    }

    @Test
    void allowsPlayerWhenWhitelistNonEmptyAndPlayerPresent() {
        assertTrue(CommandsSpyPlayerFilter.shouldLogPlayer(PLAYER, List.of(), List.of("ultra_mc")),
                "Player should be allowed when present in non-empty whitelist (case-insensitive)");
    }
}
```

- [ ] **Step 3: Create the implementation**

Create `src/main/java/pl/m2x/commandsspy/CommandsSpyPlayerFilter.java`:
```java
package pl.m2x.commandsspy;

import java.util.List;
import java.util.Locale;

public class CommandsSpyPlayerFilter {
    private CommandsSpyPlayerFilter() {}

    public static boolean shouldLogPlayer(String playerName, List<String> playersBlacklist, List<String> playersWhitelist) {
        String name = playerName.toLowerCase(Locale.ROOT);
        if (playersBlacklist.stream().anyMatch(n -> n.toLowerCase(Locale.ROOT).equals(name))) {
            return false;
        }
        if (playersWhitelist.isEmpty()) {
            return true;
        }
        return playersWhitelist.stream().anyMatch(n -> n.toLowerCase(Locale.ROOT).equals(name));
    }
}
```

- [ ] **Step 4: Run tests**

```bash
./gradlew test --tests "pl.m2x.commandsspy.CommandsSpyPlayerFilterTest" 2>&1 | tail -20
```
Expected: 4 tests PASS

- [ ] **Step 5: Commit**

```bash
git add build.gradle src/main/java/pl/m2x/commandsspy/CommandsSpyPlayerFilter.java src/test/java/pl/m2x/commandsspy/CommandsSpyPlayerFilterTest.java
git commit -m "feat: add CommandsSpyPlayerFilter with playersBlacklist/playersWhitelist"
```

---

### Task 4: Wire player filter into CommandManagerMixin

**Files:**
- Modify: `src/main/java/pl/m2x/commandsspy/mixin/CommandManagerMixin.java`

**Interfaces:**
- Consumes: `CommandsSpyPlayerFilter.shouldLogPlayer(String, List<String>, List<String>)`
- Consumes: `CommandsSpy.CONFIG.playersBlacklist`, `CommandsSpy.CONFIG.playersWhitelist`

- [ ] **Step 1: Update CommandManagerMixin.java**

```java
package pl.m2x.commandsspy.mixin;

import com.mojang.brigadier.ParseResults;
import net.minecraft.server.command.CommandManager;
import net.minecraft.server.command.ServerCommandSource;
import net.minecraft.server.network.ServerPlayerEntity;
import net.minecraft.entity.Entity;
import org.spongepowered.asm.mixin.Mixin;
import org.spongepowered.asm.mixin.injection.At;
import org.spongepowered.asm.mixin.injection.Inject;
import org.spongepowered.asm.mixin.injection.callback.CallbackInfo;
import pl.m2x.commandsspy.CommandsSpy;
import pl.m2x.commandsspy.CommandsSpyCommand;
import pl.m2x.commandsspy.CommandsSpyPlayerFilter;

@Mixin(CommandManager.class)
public class CommandManagerMixin {
    @SuppressWarnings({ "PMD.UnusedPrivateMethod", "PMD.UnusedFormalParameter" })
    @Inject(method = "execute", at = @At("HEAD"))
    private void onCommandExecute(ParseResults<ServerCommandSource> parseResults, String fullCommand, CallbackInfo ci) {
        String command = CommandsSpyCommand.getCommand(fullCommand);
        if (CommandsSpy.BLACKLIST.isBlacklisted(command)) {
            return;
        }

        ServerCommandSource source = parseResults.getContext().getSource();
        Entity entity = source.getEntity();

        String commandToLog = CommandsSpy.CONFIG.logArguments ? fullCommand : command;

        if (entity instanceof ServerPlayerEntity player) {
            String playerName = player.getName().getString();
            if (!CommandsSpyPlayerFilter.shouldLogPlayer(playerName, CommandsSpy.CONFIG.playersBlacklist, CommandsSpy.CONFIG.playersWhitelist)) {
                return;
            }
            CommandsSpy.logCommand(commandToLog, "Player: " + playerName);
        } else {
            CommandsSpy.logCommand(commandToLog, source.getName());
        }
    }
}
```

- [ ] **Step 2: Commit**

```bash
git add src/main/java/pl/m2x/commandsspy/mixin/CommandManagerMixin.java
git commit -m "feat: wire player filter into command logging"
```

---

### Task 5: Update documentation

**Files:**
- Modify: `MOD.md`

- [ ] **Step 1: Update MOD.md config section**

Replace the `## Configuration` section to document the new field names:

```markdown
## Configuration
### Config file `config/commands-spy.json`
#### Initial config
```
{
"commandsBlacklist": [],
"playersBlacklist": [],
"playersWhitelist": [],
"logArguments": false
}
```

#### commandsBlacklist
To prevent logging certain commands, add command names to `commandsBlacklist`.
Example - to maintain privacy of players' conversations, you can avoid logging commands `tell` and `t`.
```
"commandsBlacklist": ["tell", "t"]
```

#### playersBlacklist
Player usernames in `playersBlacklist` are excluded from logging. Blacklist wins over whitelist.
```
"playersBlacklist": ["spammer", "griefer"]
```

#### playersWhitelist
If non-empty, player-sourced commands are logged **only** for usernames in `playersWhitelist` (unless also in `playersBlacklist`).
```
"playersWhitelist": ["Ultra_MC", "Admin2"]
```

#### log arguments
Command arguments are not logged by default: `"logArguments": false`.
Example: `/a b c` - `[CommandsSpy] [Player: Ultra_MC] a`

To log arguments of all commands, use `"logArguments": true`.
Example: `/a b c` - `[CommandsSpy] [Player: Ultra_MC] a b c`
```

- [ ] **Step 2: Commit**

```bash
git add MOD.md
git commit -m "docs: update config docs for commandsBlacklist, playersBlacklist, playersWhitelist"
```
