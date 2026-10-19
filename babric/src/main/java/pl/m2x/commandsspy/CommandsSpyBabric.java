package pl.m2x.commandsspy;

import net.fabricmc.api.ModInitializer;
import net.fabricmc.loader.api.FabricLoader;

/**
 * Babric entrypoint. The only Babric class that touches a loader API; everything it
 * needs lives in {@link CommandsSpy}, which stays loader-agnostic so all five loaders
 * compile the same shared core. Mirrors CommandsSpyFabric, which does the same job on
 * Fabric and Quilt.
 *
 * <p>It also owns {@link #normalize(String)}, because Beta 1.7.3 is the one platform
 * whose two command seams disagree about the leading slash. Normalizing here rather
 * than in the shared core keeps that quirk from reaching the other four loaders.
 */
public class CommandsSpyBabric implements ModInitializer {

	@Override
	public void onInitialize() {
		CommandsSpy.init();
		CommandsSpy.startMetrics("Babric", modVersion("commandsspy"), modVersion("minecraft"));
	}

	/**
	 * Strips one leading {@code /} if present.
	 *
	 * <p>{@code ServerPlayNetworkHandler#handleCommand} receives the raw chat line with
	 * its slash intact ({@code "/me waves"}); {@code ServerCommandHandler#executeCommand}
	 * receives the bare line ({@code "save-all"}). Every other loader hands
	 * {@link CommandsSpy#handleCommand} a bare name, so both seams are brought to that
	 * shape here and the console output reads identically across all five loaders.
	 *
	 * <p>Exactly one slash is removed, so {@code "//me"} normalizes to {@code "/me"}
	 * rather than to {@code "me"} - a doubled slash is a distinct input, not a typo to
	 * silently repair.
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

	/**
	 * Version strings for the bStats charts, straight from the loader's own metadata.
	 *
	 * @param modId the mod to look up, "commandsspy" or "minecraft"
	 * @return the friendly version string, or null when the container is absent
	 */
	@SuppressWarnings("PMD.AvoidCatchingGenericException") // a chart value is worth less than a booted mod
	private static String modVersion(final String modId) {
		try {
			return FabricLoader.getInstance().getModContainer(modId)
					.map(container -> container.getMetadata().getVersion().getFriendlyString())
					.orElse(null);
		} catch (RuntimeException e) {
			// Evaluated at the call site, i.e. outside CommandsSpy.startMetrics' own guard:
			// an unknown chart value is worth less than a booted mod.
			CommandsSpy.LOGGER.warn("[CommandsSpy] Could not read the version of {}.", modId, e);
			return null;
		}
	}
}
