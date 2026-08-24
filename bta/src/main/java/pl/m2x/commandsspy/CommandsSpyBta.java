package pl.m2x.commandsspy;

import net.fabricmc.api.ModInitializer;

/**
 * BTA entrypoint. The only BTA class that touches a loader API; everything it needs
 * lives in {@link CommandsSpy}, which stays loader-agnostic so all six loaders compile
 * the same shared core. Mirrors CommandsSpyFabric and CommandsSpyBabric.
 *
 * <p>It also owns {@link #normalize(String)}. BTA strips the leading slash itself
 * before dispatching - {@code PacketHandlerServer#handleSlashCommand} calls
 * {@code CommandManager.execute(message.substring(1), source)} - so the seam already
 * delivers a bare line. Normalizing anyway keeps the mod's output identical if a later
 * BTA release stops stripping, and keeps this platform's quirk out of the shared core.
 */
public class CommandsSpyBta implements ModInitializer {

	@Override
	public void onInitialize() {
		CommandsSpy.init();
	}

	/**
	 * Strips one leading {@code /} if present.
	 *
	 * <p>Exactly one slash is removed, so {@code "//me"} normalizes to {@code "/me"}
	 * rather than to {@code "me"} - a doubled slash is a distinct input, not a typo to
	 * silently repair. Same contract as {@code CommandsSpyBabric.normalize}.
	 *
	 * @param rawCommandLine the line as the seam delivered it, never null
	 * @return the line without its leading slash
	 */
	public static String normalize(final String rawCommandLine) {
		if (rawCommandLine.startsWith("/")) {
			return rawCommandLine.substring(1);
		}
		return rawCommandLine;
	}
}
