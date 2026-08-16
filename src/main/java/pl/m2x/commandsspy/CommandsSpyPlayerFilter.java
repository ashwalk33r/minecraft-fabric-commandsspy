package pl.m2x.commandsspy;

import java.util.List;
import java.util.Locale;

public class CommandsSpyPlayerFilter {
    private CommandsSpyPlayerFilter() {
    }

    public static boolean shouldLogPlayer(String playerName, List<String> blacklist, List<String> whitelist) {
        String normalizedPlayerName = playerName.toLowerCase(Locale.ROOT);

        if (blacklist.stream()
                .map(name -> name.toLowerCase(Locale.ROOT))
                .anyMatch(normalizedPlayerName::equals)) {
            return false;
        }

        if (whitelist.isEmpty()) {
            return true;
        }

        return whitelist.stream()
                .map(name -> name.toLowerCase(Locale.ROOT))
                .anyMatch(normalizedPlayerName::equals);
    }
}
