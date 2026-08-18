package pl.m2x.commandsspy;

import org.apache.logging.log4j.Level;

/**
 * Shared setup/teardown for tests that touch CommandsSpy's static state.
 *
 * <p>CommandsSpy.CONFIG and CommandsSpy.BLACKLIST are public static finals initialised
 * once at class-load time, and BLACKLIST captures the very list instance that
 * CONFIG.blacklist points at. Two consequences drive everything here:
 *
 * <ul>
 *   <li>The blacklist must be mutated <b>in place</b> (add/clear). Reassigning
 *       CommandsSpy.CONFIG.blacklist swaps in a new list that BLACKLIST does not see,
 *       so the change has no effect on handleCommand - which is exactly the trap the
 *       feasibility probe fell into.</li>
 *   <li>State leaks between tests unless it is reset. resetState() is called from every
 *       test class's @BeforeEach so each test starts from empty-blacklist /
 *       logArguments=false regardless of ordering, and regardless of any
 *       config/commands-spy.json a previous run left on disk (FabricLoader's config dir
 *       under fabric-loader-junit is a real directory and CommandsSpyConfig.load()
 *       writes into it).</li>
 * </ul>
 *
 * <p>No production accessor is needed: the fields are already public and the list is
 * already mutable, so tests get full control without touching the mod's source.
 */
final class CommandsSpyTestSupport {

    private CommandsSpyTestSupport() {
    }

    /** Restore CommandsSpy's static config to its documented defaults. */
    static void resetState() {
        CommandsSpy.CONFIG.blacklist.clear();
        CommandsSpy.CONFIG.logArguments = false;
    }

    /** Attach a fresh capturing appender to CommandsSpy.LOGGER and return it. */
    static CapturingAppender attachAppender() {
        final CapturingAppender appender = new CapturingAppender();
        appender.start();
        CommandsSpy.LOGGER.addAppender(appender);
        CommandsSpy.LOGGER.setLevel(Level.ALL);
        return appender;
    }

    /** Detach an appender previously returned by attachAppender(). */
    static void detachAppender(final CapturingAppender appender) {
        CommandsSpy.LOGGER.removeAppender(appender);
        appender.stop();
    }
}
