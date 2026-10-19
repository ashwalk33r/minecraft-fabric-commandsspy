package pl.m2x.commandsspy;

import org.apache.logging.log4j.Level;
import org.apache.logging.log4j.core.Logger;

import java.io.IOException;
import java.nio.file.Files;

/**
 * Shared setup/teardown for tests touching CommandsSpy's static state.
 *
 * <p>BLACKLIST captures the very list instance CONFIG.blacklist points at, so the
 * blacklist must be mutated in place (add/clear) - reassigning CONFIG.blacklist has
 * no effect on handleCommand. resetState() runs from every @BeforeEach so tests are
 * order-independent and immune to a config/commands-spy.json left on disk.
 */
final class CommandsSpyTestSupport {

    private CommandsSpyTestSupport() {
    }

    static void resetState() {
        CommandsSpy.CONFIG.blacklist.clear();
        CommandsSpy.CONFIG.logArguments = false;
    }

    /**
     * The startMetrics guard is static state with the same problem as CONFIG/BLACKLIST, and
     * config/bStats/config.json is a real file in the project dir under fabric-loader-junit.
     * Both are cleared so metrics tests are order-independent.
     *
     * @throws IOException if the bStats config file exists and cannot be deleted
     */
    static void resetMetricsState() throws IOException {
        CommandsSpy.resetMetricsForTests();
        Files.deleteIfExists(CommandsSpyMetrics.CONFIG_PATH);
    }

    static CapturingAppender attachAppender() {
        final CapturingAppender appender = new CapturingAppender();
        appender.start();
        // CommandsSpy.LOGGER is deliberately the log4j-API type (see CommandsSpy.java), so the
        // core-only appender/level API is reached by a cast here instead. log4j-core is on the
        // test classpath on every target; CapturingAppender already extends it.
        final Logger coreLogger = (Logger) CommandsSpy.LOGGER;
        coreLogger.addAppender(appender);
        coreLogger.setLevel(Level.ALL);
        return appender;
    }

    static void detachAppender(final CapturingAppender appender) {
        ((Logger) CommandsSpy.LOGGER).removeAppender(appender);
        appender.stop();
    }
}
