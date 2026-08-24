package pl.m2x.commandsspy;

import net.fabricmc.api.ModInitializer;
import net.fabricmc.loader.api.FabricLoader;

/**
 * Fabric/Quilt entrypoint. The only class in this mod that touches a loader API;
 * everything it needs lives in {@link CommandsSpy}, which stays loader-agnostic so
 * the Forge build can compile the same shared core. See the wiki,
 * Version-Boundaries-And-Root-Causes -> "The loader seam in the shared core".
 */
public class CommandsSpyFabric implements ModInitializer {
	@Override
	public void onInitialize() {
		CommandsSpy.init();
		CommandsSpy.startMetrics(
				FabricLoader.getInstance().isModLoaded("quilt_loader") ? "Quilt" : "Fabric",
				modVersion("commandsspy"), modVersion("minecraft"));
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
