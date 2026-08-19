package pl.m2x.commandsspy;

import org.apache.logging.log4j.Level;

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

    static CapturingAppender attachAppender() {
        final CapturingAppender appender = new CapturingAppender();
        appender.start();
        CommandsSpy.LOGGER.addAppender(appender);
        CommandsSpy.LOGGER.setLevel(Level.ALL);
        return appender;
    }

    static void detachAppender(final CapturingAppender appender) {
        CommandsSpy.LOGGER.removeAppender(appender);
        appender.stop();
    }
}
