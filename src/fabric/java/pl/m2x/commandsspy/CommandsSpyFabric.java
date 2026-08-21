package pl.m2x.commandsspy;

import net.fabricmc.api.ModInitializer;

/**
 * Fabric/Quilt entrypoint. The only class in this mod that touches a loader API;
 * everything it needs lives in {@link CommandsSpy}, which stays loader-agnostic so
 * the Forge build can compile the same shared core. See the wiki, Supported-Versions.
 */
public class CommandsSpyFabric implements ModInitializer {
	@Override
	public void onInitialize() {
		CommandsSpy.init();
	}
}
