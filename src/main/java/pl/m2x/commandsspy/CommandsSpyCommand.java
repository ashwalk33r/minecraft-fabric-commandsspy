package pl.m2x.commandsspy;

public class CommandsSpyCommand {
	public static String getCommand(String fullCommand) {
		int spaceIndex = fullCommand.indexOf(' ');
		return spaceIndex > 0 ? fullCommand.substring(0, spaceIndex) : fullCommand;
	}
}
