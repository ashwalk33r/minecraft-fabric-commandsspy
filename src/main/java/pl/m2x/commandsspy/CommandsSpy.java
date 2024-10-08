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
}
