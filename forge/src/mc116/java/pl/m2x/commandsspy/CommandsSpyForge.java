package pl.m2x.commandsspy;

import com.mojang.brigadier.ParseResults;
import net.minecraft.command.CommandSource;
import net.minecraft.entity.Entity;
import net.minecraft.entity.player.ServerPlayerEntity;
import net.minecraftforge.common.MinecraftForge;
import net.minecraftforge.event.CommandEvent;
import net.minecraftforge.fml.ModList;
import net.minecraftforge.fml.common.Mod;

/**
 * Forge entrypoint for the pre-1.17 SRG era (issue #30). Same event hook as the
 * default entrypoint, two era differences:
 *
 * <ul>
 * <li>Pre-1.17 dev-time CLASS names are the MCP ones ({@code CommandSource},
 * {@code ServerPlayerEntity}), not the Mojang ones the 1.17+ targets compile
 * against -- Forge's own {@code CommandEvent} signature references them, so a
 * mapping swap cannot bridge this; it needs this sibling source (measured, #30).
 * Member names ARE Mojang official at compile time; the renamer maps them to
 * the runtime's {@code func_xxxxx_} SRG ids (class names already match),
 * exactly like legacy.</li>
 * <li>Classic check-and-cast instead of a pattern-matching {@code instanceof}:
 * this target compiles with {@code --release 8} (1.16.x servers run Java 8,
 * which predates Java 16 pattern matching).</li>
 * </ul>
 */
@Mod("commandsspy")
public class CommandsSpyForge {
	public CommandsSpyForge() {
		CommandsSpy.init();
		CommandsSpy.startMetrics("Forge", modVersion("commandsspy"), modVersion("minecraft"));
		MinecraftForge.EVENT_BUS.addListener(CommandsSpyForge::onCommand);
	}

	private static void onCommand(CommandEvent event) {
		ParseResults<CommandSource> parse = event.getParseResults();
		CommandSource source = parse.getContext().getSource();
		Entity entity = source.getEntity();
		// Same raw-line recovery as the default entrypoint; see its comment.
		String fullCommand = parse.getReader().getString();

		if (entity instanceof ServerPlayerEntity) {
			ServerPlayerEntity player = (ServerPlayerEntity) entity;
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
