package pl.m2x.commandsspy.neoforge;

import com.mojang.brigadier.ParseResults;
import net.minecraft.commands.CommandSourceStack;
import net.minecraft.server.level.ServerPlayer;
import net.minecraft.world.entity.Entity;
import net.neoforged.fml.common.Mod;
import net.neoforged.neoforge.common.NeoForge;
import net.neoforged.neoforge.event.CommandEvent;
import pl.m2x.commandsspy.CommandsSpy;

/**
 * NeoForge entrypoint. No mixin: NeoForge fires {@link CommandEvent} from
 * {@code Commands.performCommand}, which is the exact instruction the Fabric
 * builds' {@code CommandManagerMixin} injects at {@code @At("HEAD")} of. Same
 * hook point, same coverage (player, console, RCON, command block), same blind
 * spot (datapack functions and {@code /execute run} sub-commands, which have
 * gone through {@code Commands.executeCommandInContext} since 1.20.2).
 *
 * <p>Using the published event instead of a mixin also decouples this build from
 * Minecraft's own method-signature churn — the thing that already forced four
 * separate source sets on the Fabric side.
 */
@Mod("commandsspy")
public final class CommandsSpyNeoForge {

	/**
	 * Registers the listener explicitly rather than via {@code @EventBusSubscriber}.
	 *
	 * <p>The annotation infers which bus to dispatch on, and that inference has
	 * changed across the NeoForge lines this jar family targets; a wrong guess
	 * fails <em>silently</em> — no listener, no error, no log line. An explicit
	 * {@code addListener} on {@link NeoForge#EVENT_BUS} is unambiguous and
	 * compiled.
	 *
	 * <p>The banner is emitted here, not from {@code FMLCommonSetupEvent} (which
	 * is dispatched off the main thread, so its position in the log is
	 * non-deterministic) and not from {@code ServerStartingEvent} (later, and
	 * conceptually re-firable). The constructor runs exactly once per launch,
	 * before any server exists and therefore before any command can execute —
	 * the closest analogue to Fabric's {@code onInitialize()}. It also forces the
	 * config load at boot, so a malformed {@code commands-spy.json} fails at
	 * startup here exactly as it does on Fabric.
	 */
	public CommandsSpyNeoForge() {
		NeoForge.EVENT_BUS.addListener(CommandsSpyNeoForge::onCommand);
		CommandsSpy.init();
	}

	private static void onCommand(final CommandEvent event) {
		final ParseResults<CommandSourceStack> parseResults = event.getParseResults();
		// The event carries no raw command string; the reader's backing string is
		// what the Fabric mixins receive as their `fullCommand` parameter.
		final String fullCommand = parseResults.getReader().getString();
		final CommandSourceStack source = parseResults.getContext().getSource();
		final Entity entity = source.getEntity();

		if (entity instanceof ServerPlayer player) {
			CommandsSpy.handleCommand(fullCommand, true, player.getName().getString());
		} else {
			CommandsSpy.handleCommand(fullCommand, false, source.getTextName());
		}
	}
}
