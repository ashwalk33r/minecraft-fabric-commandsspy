package pl.m2x.commandsspy;

import com.google.gson.JsonSyntaxException;
import net.fabricmc.loader.api.FabricLoader;
import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;

import java.io.IOException;
import java.nio.charset.StandardCharsets;
import java.nio.file.Files;
import java.nio.file.Path;
import java.util.Arrays;
import java.util.Collections;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertNull;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTrue;

/**
 * Suite for CommandsSpyConfig.load()/save(): the on-disk commands-spy.json
 * variants an operator can produce. CommandsSpy.CONFIG is loaded once at
 * class-init before any test runs, so these tests call load()/save() directly
 * against the same real config directory and never touch that static field -
 * see docs/testing.md.
 */
class CommandsSpyConfigTest {

    private static final Path CONFIG_PATH =
            FabricLoader.getInstance().getConfigDir().resolve("commands-spy.json");
    private static final String TELL = "tell";

    @BeforeEach
    void setUp() throws IOException {
        Files.deleteIfExists(CONFIG_PATH);
    }

    @AfterEach
    void tearDown() throws IOException {
        Files.deleteIfExists(CONFIG_PATH);
    }

    private void writeConfig(String json) throws IOException {
        Files.write(CONFIG_PATH, json.getBytes(StandardCharsets.UTF_8));
    }

    @Test
    void createsDefaultConfigFileWhenNoneExists() throws IOException {
        assertFalse(Files.exists(CONFIG_PATH));

        CommandsSpyConfig config = CommandsSpyConfig.load();

        assertTrue(Files.exists(CONFIG_PATH));
        assertEquals(Collections.emptyList(), config.blacklist);
        assertFalse(config.logArguments);
    }

    @Test
    void loadsAnExistingFileWithBothFieldsSet() throws IOException {
        writeConfig("{\"blacklist\": [\"tell\", \"t\"], \"logArguments\": true}");

        CommandsSpyConfig config = CommandsSpyConfig.load();

        assertEquals(Arrays.asList(TELL, "t"), config.blacklist);
        assertTrue(config.logArguments);
    }

    @Test
    void defaultsLogArgumentsToFalseWhenTheKeyIsMissing() throws IOException {
        writeConfig("{\"blacklist\": [\"tell\"]}");

        CommandsSpyConfig config = CommandsSpyConfig.load();

        assertFalse(config.logArguments);
        assertEquals(Collections.singletonList(TELL), config.blacklist);
    }

    @Test
    void defaultsBlacklistToAnEmptyListWhenTheKeyIsMissing() throws IOException {
        writeConfig("{\"logArguments\": true}");

        CommandsSpyConfig config = CommandsSpyConfig.load();

        assertTrue(config.logArguments);
        assertEquals(Collections.emptyList(), config.blacklist);
    }

    @Test
    void anExplicitNullBlacklistLoadsAsNull() throws IOException {
        // FINDING: Gson overwrites the field to null instead of keeping the
        // empty-list default from the constructor; see
        // blacklistFromANullListNpesOnLookup for the downstream failure.
        writeConfig("{\"blacklist\": null, \"logArguments\": false}");

        CommandsSpyConfig config = CommandsSpyConfig.load();

        assertNull(config.blacklist);
    }

    @Test
    void blacklistFromANullListNpesOnLookup() throws IOException {
        // FINDING: CommandsSpyBlacklist.isBlacklisted calls
        // blacklist.contains(...) directly with no null check, so the very
        // next command executed after loading this config throws.
        writeConfig("{\"blacklist\": null}");
        CommandsSpyConfig config = CommandsSpyConfig.load();
        CommandsSpyBlacklist blacklist = new CommandsSpyBlacklist(config.blacklist);

        assertThrows(NullPointerException.class, () -> blacklist.isBlacklisted("say"));
    }

    @Test
    void malformedJsonThrowsOnLoad() throws IOException {
        // FINDING: load()'s try/catch only wraps IOException; a JSON syntax
        // error surfaces as an uncaught JsonSyntaxException. At the real
        // CommandsSpy.CONFIG static-init call site this aborts mod
        // classloading entirely rather than falling back to defaults.
        writeConfig("{ not valid json");

        assertThrows(JsonSyntaxException.class, CommandsSpyConfig::load);
    }

    @Test
    void loadDoesNotOverwriteAnExistingFile() throws IOException {
        writeConfig("{\"blacklist\": [\"tell\"], \"logArguments\": true}");
        byte[] before = Files.readAllBytes(CONFIG_PATH);

        CommandsSpyConfig.load();

        assertEquals(new String(before, StandardCharsets.UTF_8),
                new String(Files.readAllBytes(CONFIG_PATH), StandardCharsets.UTF_8));
    }

    @Test
    void saveRoundTripsTheCurrentFieldsToDisk() {
        CommandsSpyConfig config = new CommandsSpyConfig();
        config.blacklist.add(TELL);
        config.logArguments = true;

        config.save();
        CommandsSpyConfig reloaded = CommandsSpyConfig.load();

        assertEquals(Collections.singletonList(TELL), reloaded.blacklist);
        assertTrue(reloaded.logArguments);
    }
}
