package pl.m2x.commandsspy;

import org.junit.jupiter.api.Test;

import static org.junit.jupiter.api.Assertions.assertEquals;

/**
 * Characterization tests for CommandsSpyCommand.getCommand.
 *
 * <p>getCommand is the single splitter that decides what the blacklist is matched
 * against and what gets logged when logArguments is false. Its whole implementation is
 * {@code int i = fullCommand.indexOf(' '); return i > 0 ? fullCommand.substring(0, i) : fullCommand;}
 * - which has three behaviours that are surprising enough to be pinned down explicitly
 * (leading space, tab separator, all-blank input). These tests assert WHAT THE CODE DOES
 * today, not what it arguably should do; see the plan's Findings section.
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
        // FINDING: indexOf(' ') == 0 is not > 0, so nothing is split off and the entire
        // string - arguments included - becomes the "command name". A leading space
        // therefore bypasses blacklist matching entirely.
        assertEquals(" say hello world", CommandsSpyCommand.getCommand(" say hello world"));
    }

    @Test
    void returnsWholeInputUnchangedWhenItIsOnlySpaces() {
        // FINDING: same root cause as the leading-space case.
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
