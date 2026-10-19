package pl.m2x.commandsspy;

import com.mojang.brigadier.ParseResults;
import net.minecraft.commands.CommandSourceStack;
import net.minecraft.server.level.ServerPlayer;
import net.minecraft.world.entity.Entity;
import net.minecraftforge.event.CommandEvent;
import net.minecraftforge.fml.ModList;
import net.minecraftforge.fml.common.Mod;

/**
 * Forge entrypoint, Minecraft 1.21.6+ (Forge EventBus 7).
 *
 * <p>Same hook and body as the EventBus-6 entrypoint in {@code src/main/java}
 * — {@code CommandEvent} is still fired inside {@code Commands#performCommand}
 * with the same {@code ParseResults} — but EventBus 7 (Forge 56+) replaced the
 * global {@code MinecraftForge.EVENT_BUS} with a static per-event bus, so
 * registration is {@code CommandEvent.BUS.addListener}. A sibling source file,
 * not an edit: the EventBus-6 file must keep compiling for modern/legacy.
 */
@Mod("commandsspy")
public class CommandsSpyForge {
	public CommandsSpyForge() {
		CommandsSpy.init();
		CommandsSpy.startMetrics("Forge", modVersion("commandsspy"), modVersion("minecraft"));
		CommandEvent.BUS.addListener(CommandsSpyForge::onCommand);
	}

	private static void onCommand(CommandEvent event) {
		ParseResults<CommandSourceStack> parse = event.getParseResults();
		CommandSourceStack source = parse.getContext().getSource();
		Entity entity = source.getEntity();
		// The Fabric Mixins get the raw line as a method parameter; on Forge it is
		// recovered from the reader the parse was produced from. Risk R3 — the e2e
		// log-literal assertions are what prove these agree.
		String fullCommand = parse.getReader().getString();

		if (entity instanceof ServerPlayer player) {
			CommandsSpy.handleCommand(fullCommand, true, player.getName().getString());
		} else {
			CommandsSpy.handleCommand(fullCommand, false, source.getTextName());
		}
	}

	/**
	 * Version strings for the bStats charts, straight from the loader's own mod list.
	 *
	 * @param modId the mod to look up, "commandsspy" or "minecraft"
	 * @return the version string, or null when the container is absent
	 */
	@SuppressWarnings("PMD.AvoidCatchingGenericException") // a chart value is worth less than a booted mod
	private static String modVersion(final String modId) {
		try {
			return ModList.get().getModContainerById(modId)
					.map(container -> container.getModInfo().getVersion().toString())
					.orElse(null);
		} catch (RuntimeException e) {
			// Evaluated at the call site, i.e. outside CommandsSpy.startMetrics' own guard:
			// an unknown chart value is worth less than a booted mod.
			CommandsSpy.LOGGER.warn("[CommandsSpy] Could not read the version of {}.", modId, e);
			return null;
		}
	}
}
