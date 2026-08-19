package pl.m2x.commandsspy;

import org.junit.jupiter.api.Test;

import static org.junit.jupiter.api.Assertions.assertEquals;

/**
 * Characterization tests for CommandsSpyCommand.getCommand: they assert what the
 * code does today, not what it should do. See docs/testing.md for the known quirks.
 */
class CommandsSpyCommandTest {

    private static final String SAY_WITH_ARGS = "say hello world";

    @Test
    void returnsBareNameForCommandWithArguments() {
        assertEquals("say", CommandsSpyCommand.getCommand(SAY_WITH_ARGS));
    }

    @Test
    void keepsLeadingSlashAsPartOfTheName() {
        assertEquals("/say", CommandsSpyCommand.getCommand("/say hello world"));
    }

    @Test
    void returnsWholeInputWhenThereIsNoSpace() {
        assertEquals("list", CommandsSpyCommand.getCommand("list"));
    }

    @Test
    void splitsAtTheFirstSpaceWhenSpacesRepeat() {
        assertEquals("say", CommandsSpyCommand.getCommand("say  hello  world"));
    }

    @Test
    void dropsATrailingSpace() {
        assertEquals("say", CommandsSpyCommand.getCommand("say "));
    }

    @Test
    void returnsWholeInputUnchangedWhenItStartsWithASpace() {
        // FINDING: indexOf(' ') == 0 is not > 0, so nothing is split off and a
        // leading space bypasses blacklist matching entirely.
        assertEquals(" say hello world", CommandsSpyCommand.getCommand(" say hello world"));
    }

    @Test
    void returnsWholeInputUnchangedWhenItIsOnlySpaces() {
        assertEquals("  ", CommandsSpyCommand.getCommand("  "));
    }

    @Test
    void treatsATabAsPartOfTheCommandName() {
        // FINDING: only U+0020 is a separator; a tab-separated command is never split.
        assertEquals("say\thello", CommandsSpyCommand.getCommand("say\thello"));
    }

    @Test
    void returnsEmptyStringForEmptyInput() {
        assertEquals("", CommandsSpyCommand.getCommand(""));
    }
}
