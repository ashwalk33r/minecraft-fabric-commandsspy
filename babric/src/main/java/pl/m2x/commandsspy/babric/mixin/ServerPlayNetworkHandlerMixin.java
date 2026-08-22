package pl.m2x.commandsspy.babric.mixin;

import net.minecraft.entity.player.ServerPlayerEntity;
import net.minecraft.server.network.ServerPlayNetworkHandler;
import org.spongepowered.asm.mixin.Mixin;
import org.spongepowered.asm.mixin.Shadow;
import org.spongepowered.asm.mixin.injection.At;
import org.spongepowered.asm.mixin.injection.Inject;
import org.spongepowered.asm.mixin.injection.callback.CallbackInfo;
import pl.m2x.commandsspy.CommandsSpy;
import pl.m2x.commandsspy.CommandsSpyBabric;

/**
 * Player-typed commands on Minecraft Beta 1.7.3. There is no Brigadier here - the
 * server splits the chat line by hand - so this is a different seam from the
 * CommandManager.execute hook every other jar in this repository uses.
 *
 * <p>Injected at HEAD, before the server's own parsing, so a command the server cannot
 * resolve is logged exactly like one it can. That is MOD.md's opening claim and it is
 * asserted per-loader in the e2e leg.
 *
 * <p>The argument arrives with its leading slash intact, unlike the console seam's;
 * {@link CommandsSpyBabric#normalize} is what reconciles the two so Babric's console
 * output reads identically to every other loader's.
 *
 * <p>Attribution is {@code player.name}, a plain String: Beta 1.7.3 predates UUIDs, so
 * there is no stable identity to attribute across a rename. The field is inherited from
 * PlayerEntity, not declared on ServerPlayerEntity. Recorded on the wiki.
 */
@Mixin(ServerPlayNetworkHandler.class)
public class ServerPlayNetworkHandlerMixin {

	@Shadow
	private ServerPlayerEntity player;

	@SuppressWarnings({ "PMD.UnusedPrivateMethod", "PMD.UnusedFormalParameter" })
	@Inject(method = "handleCommand", at = @At("HEAD"))
	private void onHandleCommand(final String rawCommandLine, final CallbackInfo ci) {
		CommandsSpy.handleCommand(CommandsSpyBabric.normalize(rawCommandLine), true, this.player.name);
	}
}
