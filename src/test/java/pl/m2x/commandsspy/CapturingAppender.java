package pl.m2x.commandsspy;

import org.apache.logging.log4j.Level;
import org.apache.logging.log4j.core.LogEvent;
import org.apache.logging.log4j.core.appender.AbstractAppender;

import java.util.ArrayList;
import java.util.Collections;
import java.util.List;

/**
 * Test-only log4j appender recording every event CommandsSpy's LOGGER emits.
 * Message text and level are extracted eagerly in append(): log4j may recycle
 * LogEvent instances.
 */
final class CapturingAppender extends AbstractAppender {

    /** Named to satisfy PMD's literal rules. */
    private static final int EXACTLY_ONE = 1;

    private final List<String> capturedMessages = new ArrayList<>();
    private final List<Level> capturedLevels = new ArrayList<>();

    // Deprecated 4-arg super on purpose: mc114's log4j 2.8.1 lacks the 5-arg
    // constructor, and one test tree must compile against every era.
    @SuppressWarnings("deprecation")
    CapturingAppender() {
        super("CommandsSpyCapturingAppender", null, null, false);
    }

    @Override
    public void append(final LogEvent event) {
        capturedMessages.add(event.getMessage().getFormattedMessage());
        capturedLevels.add(event.getLevel());
    }

    List<String> messages() {
        return Collections.unmodifiableList(capturedMessages);
    }

    List<Level> levels() {
        return Collections.unmodifiableList(capturedLevels);
    }

    String onlyMessage() {
        if (capturedMessages.size() != EXACTLY_ONE) {
            throw new AssertionError(
                    "expected exactly one captured log message but got " + capturedMessages);
        }
        return capturedMessages.get(0);
    }
}
