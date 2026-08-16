package pl.m2x.commandsspy;

import java.util.List;
import java.util.Locale;

public class CommandsSpyPlayerFilter {
    private CommandsSpyPlayerFilter() {
    }

    public static boolean shouldLogPlayer(String playerName, List<String> playersBlacklist, List<String> playersWhitelist) {
        String normalizedPlayerName = playerName.toLowerCase(Locale.ROOT);

        if (playersBlacklist.stream()
                .map(name -> name.toLowerCase(Locale.ROOT))
                .anyMatch(normalizedPlayerName::equals)) {
            return false;
        }

        if (playersWhitelist.isEmpty()) {
            return true;
        }

        return playersWhitelist.stream()
                .map(name -> name.toLowerCase(Locale.ROOT))
                .anyMatch(normalizedPlayerName::equals);
    }
}
