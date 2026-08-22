package pl.m2x.commandsspy.babric.mixin;

import net.minecraft.server.command.Command;
import net.minecraft.server.command.ServerCommandHandler;
import org.spongepowered.asm.mixin.Mixin;
import org.spongepowered.asm.mixin.injection.At;
import org.spongepowered.asm.mixin.injection.Inject;
import org.spongepowered.asm.mixin.injection.callback.CallbackInfo;
import pl.m2x.commandsspy.CommandsSpy;
import pl.m2x.commandsspy.CommandsSpyBabric;

/**
 * Console and op commands on Minecraft Beta 1.7.3. The dispatcher takes a Command
 * object rather than a source and a string, so both halves are read off it here.
 *
 * <p>HEAD again, for the same reason as the player seam: an unresolvable command name
 * must be logged, not swallowed.
 *
 * <p>Beta 1.7.3 has no RCON - there is no {@code enable-rcon} key and no listener - so
 * unlike every other loader this seam carries console traffic only. The e2e leg asserts
 * that absence rather than skipping the RCON check. See the wiki, Supported Versions.
 *
 * <p>{@code commandAndArgs} arrives without a leading slash, unlike the player seam's;
 * normalize() is still applied so the two paths cannot drift apart if that ever changes.
 */
@Mixin(ServerCommandHandler.class)
public class ServerCommandHandlerMixin {

	@SuppressWarnings({ "PMD.UnusedPrivateMethod", "PMD.UnusedFormalParameter" })
	@Inject(method = "executeCommand", at = @At("HEAD"))
	private void onExecuteCommand(final Command command, final CallbackInfo ci) {
		CommandsSpy.handleCommand(
				CommandsSpyBabric.normalize(command.commandAndArgs), false, command.output.getName());
	}
}
