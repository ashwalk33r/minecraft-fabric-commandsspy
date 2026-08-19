package pl.m2x.commandsspy;

import org.junit.jupiter.api.Test;

import java.util.ArrayList;
import java.util.Arrays;
import java.util.List;

import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertTrue;

class CommandsSpyBlacklistTest {

    private static final String SAY = "say";

    private static CommandsSpyBlacklist blacklistOf(final String... entries) {
        return new CommandsSpyBlacklist(new ArrayList<>(Arrays.asList(entries)));
    }

    @Test
    void emptyBlacklistMatchesNothing() {
        assertFalse(blacklistOf().isBlacklisted(SAY));
    }

    @Test
    void matchesAnEntryExactly() {
        assertTrue(blacklistOf(SAY, "tell").isBlacklisted(SAY));
    }

    @Test
    void doesNotMatchACommandThatIsAbsent() {
        assertFalse(blacklistOf(SAY, "tell").isBlacklisted("gamemode"));
    }

    @Test
    void matchingIsCaseSensitive() {
        // FINDING: "Say" in the config file will not suppress "say".
        assertFalse(blacklistOf(SAY).isBlacklisted("Say"));
    }

    @Test
    void matchingIsExactNotPrefix() {
        // FINDING: an entry of "say" does not cover "sayonara", and an entry of
        // "say hello" never matches, because getCommand only ever yields a bare name.
        assertFalse(blacklistOf(SAY).isBlacklisted("sayonara"));
    }

    @Test
    void reflectsInPlaceMutationOfTheBackingList() {
        // Guards the suite's aliasing assumption: if this fails, resetState() is a
        // no-op and every blacklist assertion in CommandsSpyHandleCommandTest is vacuous.
        final List<String> backing = new ArrayList<>();
        final CommandsSpyBlacklist blacklist = new CommandsSpyBlacklist(backing);

        assertFalse(blacklist.isBlacklisted(SAY));
        backing.add(SAY);
        assertTrue(blacklist.isBlacklisted(SAY));
        backing.clear();
        assertFalse(blacklist.isBlacklisted(SAY));
    }
}
