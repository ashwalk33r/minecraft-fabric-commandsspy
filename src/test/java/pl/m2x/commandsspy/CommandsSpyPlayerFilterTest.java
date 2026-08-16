package pl.m2x.commandsspy;

import org.junit.jupiter.api.Test;

import java.util.List;

import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertTrue;

class CommandsSpyPlayerFilterTest {
    private static final String PLAYER = "Ultra_MC";

    @Test
    void allowsPlayerWhenWhitelistEmptyAndNotBlacklisted() {
        assertTrue(CommandsSpyPlayerFilter.shouldLogPlayer(PLAYER, List.of("Other"), List.of()),
                "Player should be logged when whitelist is empty and player is not blacklisted");
    }

    @Test
    void deniesPlayerWhenBlacklistedEvenIfWhitelisted() {
        assertFalse(CommandsSpyPlayerFilter.shouldLogPlayer(PLAYER, List.of("ultra_mc"), List.of(PLAYER)),
                "Blacklist must override whitelist");
    }

    @Test
    void deniesPlayerWhenWhitelistNonEmptyAndPlayerMissing() {
        assertFalse(CommandsSpyPlayerFilter.shouldLogPlayer(PLAYER, List.of(), List.of("SomeoneElse")),
                "Player should be denied when whitelist is non-empty and does not include the player");
    }

    @Test
    void allowsPlayerWhenWhitelistNonEmptyAndPlayerPresent() {
        assertTrue(CommandsSpyPlayerFilter.shouldLogPlayer(PLAYER, List.of(), List.of("ultra_mc")),
                "Player should be allowed when present in non-empty whitelist");
    }
}
