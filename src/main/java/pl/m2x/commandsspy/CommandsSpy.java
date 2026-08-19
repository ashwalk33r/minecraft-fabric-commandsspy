package pl.m2x.commandsspy;

import net.fabricmc.api.ModInitializer;
import org.apache.logging.log4j.LogManager;
import org.apache.logging.log4j.core.Logger;


public class CommandsSpy implements ModInitializer {
	public static final Logger LOGGER = (Logger) LogManager.getLogger("CommandsSpy");
	public static final CommandsSpyConfig CONFIG = CommandsSpyConfig.load();
	public static final CommandsSpyBlacklist BLACKLIST = new CommandsSpyBlacklist(CONFIG.blacklist);

	@Override
	public void onInitialize() {
		LOGGER.info("Loading CommandsSpy by Ultra_MC.");
	}

	public static void logCommand(String command, String source) {
		LOGGER.info("[CommandsSpy] [{}] {}", source, command);
	}

	/**
	 * Shared entry point for all per-mapping mixin hooks.
	 *
	 * @param fullCommand raw command line as received by the dispatcher
	 * @param isPlayer whether the command came from a player entity
	 * @param sourceName player display name when isPlayer, otherwise the source's
	 *                   textual name (console, RCON, function, command block)
	 */
	public static void handleCommand(String fullCommand, boolean isPlayer, String sourceName) {
		String command = CommandsSpyCommand.getCommand(fullCommand);
		if (BLACKLIST.isBlacklisted(command)) {
			return;
		}

		String commandToLog = CONFIG.logArguments ? fullCommand : command;
		String logSource = isPlayer ? "Player: " + sourceName : sourceName;
		logCommand(commandToLog, logSource);
	}
}
