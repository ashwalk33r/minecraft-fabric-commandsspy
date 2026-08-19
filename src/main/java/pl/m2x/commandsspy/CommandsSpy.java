package pl.m2x.commandsspy;

import org.apache.logging.log4j.LogManager;
import org.apache.logging.log4j.core.Logger;


/**
 * Loader-agnostic core: no Fabric, Quilt or Forge type is referenced here, so every
 * loader's entrypoint compiles against the same copy of this class.
 */
public class CommandsSpy {
	public static final Logger LOGGER = (Logger) LogManager.getLogger("CommandsSpy");
	public static final CommandsSpyConfig CONFIG = CommandsSpyConfig.load();
	public static final CommandsSpyBlacklist BLACKLIST = new CommandsSpyBlacklist(CONFIG.blacklist);

	/**
	 * Called once by each loader's entrypoint. Named (rather than left as an incidental
	 * LOGGER dereference) so it stays obvious that this call is what pulls CONFIG and
	 * BLACKLIST up at boot instead of on the first executed command.
	 */
	public static void init() {
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
