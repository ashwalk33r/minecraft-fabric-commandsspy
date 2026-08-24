package pl.m2x.commandsspy;

import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;

import java.io.IOException;
import java.nio.charset.StandardCharsets;
import java.nio.file.Files;
import java.util.UUID;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertNotNull;
import static org.junit.jupiter.api.Assertions.assertTrue;

/**
 * Offline by construction: every test either opts out before calling
 * {@link CommandsSpy#startMetrics}, or exercises the config layer alone. The
 * "bStats-Metrics" thread name is what proves no submitter was ever scheduled.
 */
class CommandsSpyMetricsTest {

    private static final String KILL_SWITCH_PROPERTY = "bstats.enabled";
    private static final String SUBMIT_THREAD = "bStats-Metrics";
    private static final String LOADER = "Fabric";
    private static final String MOD_VERSION = "1.9.0";
    private static final String MC_VERSION = "1.21.1";

    @BeforeEach
    @AfterEach
    void reset() throws IOException {
        CommandsSpyTestSupport.resetMetricsState();
    }

    @Test
    void createsADefaultConfigWithAParseableUuid() {
        final CommandsSpyMetrics.Config config = CommandsSpyMetrics.loadConfig();

        assertTrue(config.enabled);
        assertNotNull(UUID.fromString(config.serverUuid));
        assertTrue(Files.exists(CommandsSpyMetrics.CONFIG_PATH));
    }

    @Test
    void reusesTheServerUuidOnASecondLoad() {
        final String first = CommandsSpyMetrics.loadConfig().serverUuid;

        assertEquals(first, CommandsSpyMetrics.loadConfig().serverUuid);
    }

    @Test
    void theSystemPropertyKillSwitchDisablesMetrics() {
        System.setProperty(KILL_SWITCH_PROPERTY, "false");
        try {
            assertFalse(CommandsSpyMetrics.enabledByEnvironment());
        } finally {
            System.clearProperty(KILL_SWITCH_PROPERTY);
        }
        assertTrue(CommandsSpyMetrics.enabledByEnvironment());
    }

    @Test
    void aDisabledConfigStartsNoSubmitThread() throws IOException {
        Files.createDirectories(CommandsSpyMetrics.CONFIG_PATH.getParent());
        Files.write(CommandsSpyMetrics.CONFIG_PATH,
                "{\"enabled\":false,\"serverUuid\":\"3f5b8a0e-0000-4000-8000-000000000001\"}"
                        .getBytes(StandardCharsets.UTF_8));

        CommandsSpy.startMetrics(LOADER, MOD_VERSION, MC_VERSION);

        assertTrue(CommandsSpy.isMetricsStarted());
        assertFalse(submitThreadExists());
    }

    @Test
    void theKillSwitchStartsNoSubmitThreadEvenWithAnEnabledConfig() {
        System.setProperty(KILL_SWITCH_PROPERTY, "false");
        try {
            CommandsSpy.startMetrics(LOADER, MOD_VERSION, MC_VERSION);
        } finally {
            System.clearProperty(KILL_SWITCH_PROPERTY);
        }

        assertFalse(submitThreadExists());
    }

    @Test
    void aSecondStartIsANoOp() {
        System.setProperty(KILL_SWITCH_PROPERTY, "false");
        try {
            CommandsSpy.startMetrics(LOADER, MOD_VERSION, MC_VERSION);
            CommandsSpy.startMetrics(LOADER, MOD_VERSION, MC_VERSION);
        } finally {
            System.clearProperty(KILL_SWITCH_PROPERTY);
        }

        assertTrue(CommandsSpy.isMetricsStarted());
        assertFalse(submitThreadExists());
    }

    private static boolean submitThreadExists() {
        for (final Thread thread : Thread.getAllStackTraces().keySet()) {
            if (SUBMIT_THREAD.equals(thread.getName())) {
                return true;
            }
        }
        return false;
    }
}
