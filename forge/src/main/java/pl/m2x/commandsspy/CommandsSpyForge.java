package pl.m2x.commandsspy;

import com.mojang.brigadier.ParseResults;
import net.minecraft.commands.CommandSourceStack;
import net.minecraft.server.level.ServerPlayer;
import net.minecraft.world.entity.Entity;
import net.minecraftforge.common.MinecraftForge;
import net.minecraftforge.event.CommandEvent;
import net.minecraftforge.fml.common.Mod;

/**
 * Forge entrypoint, Minecraft 1.20.3-1.21.5 (Forge EventBus 6).
 *
 * <p>No Mixin: {@code CommandEvent} is fired inside {@code Commands#performCommand},
 * the exact call site the Fabric Mixins inject into, and carries the same
 * {@code ParseResults}. A public cancellable event beats an injection whose
 * descriptor has already changed shape twice in this project's lifetime.
 */
@Mod("commandsspy")
public class CommandsSpyForge {
	public CommandsSpyForge() {
		CommandsSpy.init();
		MinecraftForge.EVENT_BUS.addListener(CommandsSpyForge::onCommand);
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
}
