package pl.m2x.commandsspy;

import java.util.List;
import java.util.Locale;

public class CommandsSpyPlayerFilter {
    private CommandsSpyPlayerFilter() {}

    public static boolean shouldLogPlayer(String playerName, List<String> playersBlacklist, List<String> playersWhitelist) {
        String name = playerName.toLowerCase(Locale.ROOT);
        if (playersBlacklist.stream().anyMatch(n -> n.toLowerCase(Locale.ROOT).equals(name))) {
            return false;
        }
        if (playersWhitelist.isEmpty()) {
            return true;
        }
        return playersWhitelist.stream().anyMatch(n -> n.toLowerCase(Locale.ROOT).equals(name));
    }
}
