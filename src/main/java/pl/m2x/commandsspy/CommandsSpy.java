package pl.m2x.commandsspy;

import org.apache.logging.log4j.LogManager;
import org.apache.logging.log4j.Logger;


/**
 * Loader-agnostic core: no Fabric, Quilt or Forge type is referenced here, so every
 * loader's entrypoint compiles against the same copy of this class.
 */
public class CommandsSpy {
	// log4j-API type on purpose, NOT org.apache.logging.log4j.core.Logger. This field
	// initializer runs inside CommandsSpy's class initializer, and CommandsSpyFabricPreLaunch
	// triggers that before the game's main class is loaded. On the 1.14-1.16 band (log4j
	// 2.8.1, see build.gradle) a failed core cast there would poison the class for the JVM's
	// lifetime, so every later mixin hook would get NoClassDefFoundError rather than the mod
	// merely missing a banner. Tests that need the core API cast locally; see
	// CommandsSpyTestSupport.
	public static final Logger LOGGER = LogManager.getLogger("CommandsSpy");
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
