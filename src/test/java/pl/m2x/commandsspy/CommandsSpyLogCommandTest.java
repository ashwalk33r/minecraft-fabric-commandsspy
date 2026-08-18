package pl.m2x.commandsspy;

import org.apache.logging.log4j.Level;
import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;

import java.util.Collections;

import static org.junit.jupiter.api.Assertions.assertEquals;

/**
 * Pins down the exact wire format CommandsSpy emits: {@code [CommandsSpy] [<source>] <command>}.
 * This string is what server operators grep for and what the e2e suite matches on, so it is
 * asserted character for character rather than with contains().
 */
class CommandsSpyLogCommandTest {

    private CapturingAppender appender;

    @BeforeEach
    void setUp() {
        CommandsSpyTestSupport.resetState();
        appender = CommandsSpyTestSupport.attachAppender();
    }

    @AfterEach
    void tearDown() {
        CommandsSpyTestSupport.detachAppender(appender);
    }

    @Test
    void emitsTheExactBracketedFormat() {
        CommandsSpy.logCommand("say hello world", "Player: Steve");

        assertEquals("[CommandsSpy] [Player: Steve] say hello world", appender.onlyMessage());
    }

    @Test
    void emitsAtInfoLevel() {
        CommandsSpy.logCommand("list", "Server");

        assertEquals(Collections.singletonList(Level.INFO), appender.levels());
    }

    @Test
    void stillEmitsBothBracketsForAnEmptySource() {
        CommandsSpy.logCommand("list", "");

        assertEquals("[CommandsSpy] [] list", appender.onlyMessage());
    }
}
