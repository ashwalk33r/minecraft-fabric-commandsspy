package pl.m2x.commandsspy.bta.mixin;

import net.minecraft.core.net.command.CommandManager;
import net.minecraft.core.net.command.CommandSource;
import org.spongepowered.asm.mixin.Mixin;
import org.spongepowered.asm.mixin.injection.At;
import org.spongepowered.asm.mixin.injection.Inject;
import org.spongepowered.asm.mixin.injection.callback.CallbackInfoReturnable;
import pl.m2x.commandsspy.CommandsSpy;
import pl.m2x.commandsspy.CommandsSpyBta;

/**
 * The single command seam on BTA. Unlike vanilla Beta 1.7.3, which has no command
 * registry and needs two separate hooks, BTA carries a Brigadier dispatcher and BOTH
 * dispatch paths converge on this one method:
 *
 * <ul>
 *   <li>player-typed: {@code PacketHandlerServer#handleMessage} sees the leading slash,
 *       calls {@code handleSlashCommand}, which calls
 *       {@code getCommandManager().execute(message.substring(1), new ServerCommandSource(...))}
 *   <li>console: {@code MinecraftServer}'s command queue drains into the same method
 *       with a {@code ConsoleCommandSource}
 * </ul>
 *
 * <p>Injected at HEAD, before Brigadier parses, so a command the server cannot resolve
 * is logged exactly like one it can - that is MOD.md's opening claim, and the e2e leg
 * asserts it per loader.
 *
 * <p>{@code remap = false} because BTA ships unobfuscated: there is no intermediary
 * namespace to remap through, and the loader logs "Mappings not present!" at boot.
 *
 * <p>Attribution is {@code CommandSource#getName()} - the game's own token, reaching the
 * log through the untouched shared core. There is no player-vs-console flag on the
 * source, so the concrete type decides: a player-backed source has a non-null
 * {@code getSender()}.
 */
@Mixin(value = CommandManager.class, remap = false)
public class CommandManagerMixin {

	@SuppressWarnings({ "PMD.UnusedPrivateMethod", "PMD.UnusedFormalParameter" })
	@Inject(method = "execute", at = @At("HEAD"), remap = false)
	private void onExecute(final String commandAndArgs, final CommandSource source,
			final CallbackInfoReturnable<Integer> cir) {
		CommandsSpy.handleCommand(
				CommandsSpyBta.normalize(commandAndArgs), source.getSender() != null, source.getName());
	}
}
