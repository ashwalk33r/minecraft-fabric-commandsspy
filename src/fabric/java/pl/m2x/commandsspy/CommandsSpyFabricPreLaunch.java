package pl.m2x.commandsspy;

import net.fabricmc.loader.api.entrypoint.PreLaunchEntrypoint;

/**
 * Measures whether quilt-loader invokes the "preLaunch" entrypoint on dedicated
 * servers below Minecraft 1.18, where it provably never invokes the ModInitializer
 * "main" entrypoint. Knot runs preLaunch before the game's main class is loaded — a
 * different call site from the EntrypointPatch-injected Hooks.startServer path that
 * the "main" gap lives on, so the gap does not necessarily extend to it. See
 * the wiki, Version-Boundaries-And-Root-Causes.
 *
 * <p>This logs its OWN string, never {@link CommandsSpy#init()}'s banner. That banner
 * literal is scripts/e2e-entrypoint.sh's expected-absent tripwire for the "main" gap
 * (QUILT_ENTRYPOINT_GAP); emitting it from here would report a closed upstream bug
 * that has not closed, and turn a successful measurement into a red CI run. The literal
 * is deliberately not repeated anywhere in this file, so a source-level grep for it
 * stays a reliable audit.
 */
public class CommandsSpyFabricPreLaunch implements PreLaunchEntrypoint {
	/**
	 * Both manifests declare this entrypoint, and quilt-loader's invokePreLaunch runs two
	 * stages in a row ("pre_launch" against Quilt's own interface, then "preLaunch" against
	 * Fabric's), so more than one invocation is cheap to guard against and awkward to rule
	 * out. Not volatile and not synchronized: loader entrypoint dispatch is single-threaded,
	 * and the worst a race could produce here is one duplicate log line.
	 */
	private static boolean fired;

	@Override
	public void onPreLaunch() {
		if (fired) {
			return;
		}
		fired = true;
		// Dereferencing LOGGER runs CommandsSpy's class initializer, which is what pulls
		// CONFIG (and so config/commands-spy.json's auto-creation) and BLACKLIST up. On the
		// versions where this fires, that moves back to boot time.
		CommandsSpy.LOGGER.info("CommandsSpy preLaunch: config loaded.");
	}
}
