package pl.m2x.commandsspy;

import org.apache.logging.log4j.Level;
import org.apache.logging.log4j.core.LogEvent;
import org.apache.logging.log4j.core.appender.AbstractAppender;

import java.util.ArrayList;
import java.util.Collections;
import java.util.List;

/**
 * Test-only log4j appender that records every event CommandsSpy's LOGGER emits.
 *
 * <p>CommandsSpy.LOGGER is declared as org.apache.logging.log4j.core.Logger (the
 * implementation class, not the API interface), so an appender can be attached and
 * detached directly per test - no LoggerContext reconfiguration and no log4j2-test.xml.
 *
 * <p>Message text and level are extracted eagerly in append(): log4j may recycle
 * LogEvent instances, so holding on to the events themselves is not safe.
 */
final class CapturingAppender extends AbstractAppender {

    /** Number of messages onlyMessage() insists on; named to keep PMD's literal rules happy. */
    private static final int EXACTLY_ONE = 1;

    private final List<String> capturedMessages = new ArrayList<>();
    private final List<Level> capturedLevels = new ArrayList<>();

    // The deprecated 4-arg super is used on purpose: the mc114 target resolves the
    // 1.16.5-era log4j (2.8.1), which has neither Property.EMPTY_ARRAY nor the 5-arg
    // constructor, while every newer era still ships this constructor. One test
    // source tree has to compile against all of them.
    @SuppressWarnings("deprecation")
    CapturingAppender() {
        super("CommandsSpyCapturingAppender", null, null, false);
    }

    @Override
    public void append(final LogEvent event) {
        capturedMessages.add(event.getMessage().getFormattedMessage());
        capturedLevels.add(event.getLevel());
    }

    /** Every formatted message captured so far, in emission order. */
    List<String> messages() {
        return Collections.unmodifiableList(capturedMessages);
    }

    /** The level of every captured event, in emission order. */
    List<Level> levels() {
        return Collections.unmodifiableList(capturedLevels);
    }

    /**
     * The single captured message, failing loudly if the count is not exactly one.
     * Most CommandsSpy assertions are "exactly one line, and it reads like this".
     */
    String onlyMessage() {
        if (capturedMessages.size() != EXACTLY_ONE) {
            throw new AssertionError(
                    "expected exactly one captured log message but got " + capturedMessages);
        }
        return capturedMessages.get(0);
    }
}
