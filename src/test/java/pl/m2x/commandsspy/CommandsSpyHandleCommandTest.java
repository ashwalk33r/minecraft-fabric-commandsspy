package pl.m2x.commandsspy;

import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;

import java.util.Arrays;
import java.util.Collections;

import static org.junit.jupiter.api.Assertions.assertEquals;

/**
 * Suite for CommandsSpy.handleCommand, the shared entry point every mixin hook calls.
 * Mutate CommandsSpy.CONFIG.blacklist in place, never reassign it - see docs/testing.md.
 */
class CommandsSpyHandleCommandTest {

    private static final String SAY = "say";
    private static final String SAY_WITH_ARGS = "say hello world";
    private static final String STEVE = "Steve";
    private static final String LOGGED_SAY_FROM_STEVE = "[CommandsSpy] [Player: Steve] say";
    private static final String LIST = "list";
    private static final String SERVER = "Server";
    private static final String LOGGED_LIST_FROM_SERVER = "[CommandsSpy] [Server] list";
    private static final String RCON = "Rcon";

    private CapturingAppender appender;

    @BeforeEach
    void setUp() {
        CommandsSpyTestSupport.resetState();
        appender = CommandsSpyTestSupport.attachAppender();
    }

    @AfterEach
    void tearDown() {
        CommandsSpyTestSupport.detachAppender(appender);
        CommandsSpyTestSupport.resetState();
    }

    @Test
    void logsACommandWhenTheBlacklistIsEmpty() {
        CommandsSpy.handleCommand(SAY_WITH_ARGS, true, STEVE);

        assertEquals(LOGGED_SAY_FROM_STEVE, appender.onlyMessage());
    }

    @Test
    void logsACommandThatIsNotOnANonEmptyBlacklist() {
        CommandsSpy.CONFIG.blacklist.add("tell");
        CommandsSpy.CONFIG.blacklist.add("gamemode");

        CommandsSpy.handleCommand(SAY_WITH_ARGS, true, STEVE);

        assertEquals(LOGGED_SAY_FROM_STEVE, appender.onlyMessage());
    }

    @Test
    void suppressesABareCommandThatIsBlacklisted() {
        CommandsSpy.CONFIG.blacklist.add(SAY);

        CommandsSpy.handleCommand(SAY, true, STEVE);

        assertEquals(Collections.emptyList(), appender.messages());
    }

    @Test
    void suppressesACommandWithArgumentsBlacklistedByItsBareName() {
        CommandsSpy.CONFIG.blacklist.add(SAY);

        CommandsSpy.handleCommand(SAY_WITH_ARGS, true, STEVE);

        assertEquals(Collections.emptyList(), appender.messages());
    }

    @Test
    void doesNotSuppressWhenTheBlacklistEntryIncludesArguments() {
        // FINDING: the blacklist is matched against the bare name only, so a config
        // entry of "say hello" can never match anything.
        CommandsSpy.CONFIG.blacklist.add("say hello");

        CommandsSpy.handleCommand(SAY_WITH_ARGS, true, STEVE);

        assertEquals(LOGGED_SAY_FROM_STEVE, appender.onlyMessage());
    }

    @Test
    void suppressesRegardlessOfTheLogArgumentsPreference() {
        CommandsSpy.CONFIG.logArguments = true;
        CommandsSpy.CONFIG.blacklist.add(SAY);

        CommandsSpy.handleCommand(SAY_WITH_ARGS, true, STEVE);

        assertEquals(Collections.emptyList(), appender.messages());
    }

    @Test
    void aLeadingSpaceBypassesTheBlacklist() {
        // FINDING: a leading space makes getCommand return the whole string, so the
        // blacklist never matches; see CommandsSpyCommandTest.
        CommandsSpy.CONFIG.blacklist.add(SAY);

        CommandsSpy.handleCommand(" say hello world", true, STEVE);

        assertEquals("[CommandsSpy] [Player: Steve]  say hello world", appender.onlyMessage());
    }

    @Test
    void suppressesACommandListedAmongOtherBlacklistEntries() {
        CommandsSpy.CONFIG.blacklist.add(LIST);
        CommandsSpy.CONFIG.blacklist.add("gamemode");
        CommandsSpy.CONFIG.blacklist.add("save-all");

        CommandsSpy.handleCommand("save-all", false, RCON);
        CommandsSpy.handleCommand("seed", false, RCON);

        assertEquals(Collections.singletonList("[CommandsSpy] [Rcon] seed"), appender.messages());
    }

    @Test
    void logsOnlyTheBareNameWhenLogArgumentsIsFalse() {
        CommandsSpy.CONFIG.logArguments = false;

        CommandsSpy.handleCommand(SAY_WITH_ARGS, true, STEVE);

        assertEquals(LOGGED_SAY_FROM_STEVE, appender.onlyMessage());
    }

    @Test
    void logsTheFullCommandWhenLogArgumentsIsTrue() {
        CommandsSpy.CONFIG.logArguments = true;

        CommandsSpy.handleCommand(SAY_WITH_ARGS, true, STEVE);

        assertEquals("[CommandsSpy] [Player: Steve] say hello world", appender.onlyMessage());
    }

    @Test
    void logsTheSameTextEitherWayForAnArgumentlessCommand() {
        CommandsSpy.CONFIG.logArguments = true;

        CommandsSpy.handleCommand(LIST, false, SERVER);

        assertEquals(LOGGED_LIST_FROM_SERVER, appender.onlyMessage());
    }

    @Test
    void dropsTheTrailingSpaceWhenLogArgumentsIsFalse() {
        CommandsSpy.CONFIG.logArguments = false;

        CommandsSpy.handleCommand("say ", true, STEVE);

        assertEquals(LOGGED_SAY_FROM_STEVE, appender.onlyMessage());
    }

    @Test
    void prefixesAPlayerSourceWithPlayer() {
        CommandsSpy.handleCommand(LIST, true, STEVE);

        assertEquals("[CommandsSpy] [Player: Steve] list", appender.onlyMessage());
    }

    @Test
    void leavesTheServerConsoleSourceUnprefixed() {
        CommandsSpy.handleCommand(LIST, false, SERVER);

        assertEquals(LOGGED_LIST_FROM_SERVER, appender.onlyMessage());
    }

    @Test
    void leavesTheRconSourceUnprefixed() {
        CommandsSpy.handleCommand(LIST, false, RCON);

        assertEquals("[CommandsSpy] [Rcon] list", appender.onlyMessage());
    }

    @Test
    void leavesAFunctionSourceUnprefixed() {
        CommandsSpy.handleCommand(LIST, false, "Function macro:test");

        assertEquals("[CommandsSpy] [Function macro:test] list", appender.onlyMessage());
    }

    @Test
    void leavesACommandBlockSourceUnprefixed() {
        CommandsSpy.handleCommand(LIST, false, "@");

        assertEquals("[CommandsSpy] [@] list", appender.onlyMessage());
    }

    @Test
    void appliesThePlayerPrefixPurelyFromTheIsPlayerFlag() {
        CommandsSpy.handleCommand(LIST, true, SERVER);

        assertEquals("[CommandsSpy] [Player: Server] list", appender.onlyMessage());
    }

    @Test
    void doesNotPrefixANonPlayerSourceNamedLikeAPlayer() {
        CommandsSpy.handleCommand(LIST, false, STEVE);

        assertEquals("[CommandsSpy] [Steve] list", appender.onlyMessage());
    }

    @Test
    void twoPlayersLogUnderTheirOwnNames() {
        CommandsSpy.handleCommand("/gamemode creative", true, "Alice");
        CommandsSpy.handleCommand("/tp 0 0 0", true, "Bob");

        assertEquals(
                Arrays.asList(
                        "[CommandsSpy] [Player: Alice] /gamemode",
                        "[CommandsSpy] [Player: Bob] /tp"),
                appender.messages());
    }

    @Test
    void logsAnEmptyCommandWhenNothingIsBlacklisted() {
        CommandsSpy.handleCommand("", false, SERVER);

        assertEquals("[CommandsSpy] [Server] ", appender.onlyMessage());
    }

    @Test
    void anEmptyCommandCanItselfBeBlacklisted() {
        CommandsSpy.CONFIG.blacklist.add("");

        CommandsSpy.handleCommand("", false, SERVER);

        assertEquals(Collections.emptyList(), appender.messages());
    }

    @Test
    void emitsExactlyOneLineForEachAcceptedCall() {
        CommandsSpy.handleCommand(LIST, false, SERVER);
        CommandsSpy.handleCommand("seed", false, RCON);

        assertEquals(
                Arrays.asList(LOGGED_LIST_FROM_SERVER, "[CommandsSpy] [Rcon] seed"),
                appender.messages());
    }
}
