package pl.m2x.commandsspy;

import java.util.List;

public class CommandsSpyBlacklist {
    private final List<String> blacklist;

    public CommandsSpyBlacklist(List<String> blacklist) {
        this.blacklist = blacklist;
    }

    public boolean isBlacklisted(String command) {
        return blacklist.contains(command);
    }
}
