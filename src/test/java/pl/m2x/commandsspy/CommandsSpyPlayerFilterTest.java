package pl.m2x.commandsspy;

import org.junit.jupiter.api.Test;

import java.util.List;

import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertTrue;

class CommandsSpyPlayerFilterTest {
    @Test
    void allowsPlayerWhenWhitelistEmptyAndNotBlacklisted() {
        assertTrue(CommandsSpyPlayerFilter.shouldLogPlayer("Ultra_MC", List.of("Other"), List.of()));
    }

    @Test
    void deniesPlayerWhenBlacklistedEvenIfWhitelisted() {
        assertFalse(CommandsSpyPlayerFilter.shouldLogPlayer("Ultra_MC", List.of("ultra_mc"), List.of("Ultra_MC")));
    }

    @Test
    void deniesPlayerWhenWhitelistNonEmptyAndPlayerMissing() {
        assertFalse(CommandsSpyPlayerFilter.shouldLogPlayer("Ultra_MC", List.of(), List.of("SomeoneElse")));
    }

    @Test
    void allowsPlayerWhenWhitelistNonEmptyAndPlayerPresent() {
        assertTrue(CommandsSpyPlayerFilter.shouldLogPlayer("Ultra_MC", List.of(), List.of("ultra_mc")));
    }
}
