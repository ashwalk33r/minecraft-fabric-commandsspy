package pl.m2x.commandsspy.mixin;

import net.minecraft.entity.Entity;
import net.minecraft.server.command.CommandManager;
import net.minecraft.server.command.ServerCommandSource;
import net.minecraft.server.network.ServerPlayerEntity;
import org.spongepowered.asm.mixin.Mixin;
import org.spongepowered.asm.mixin.injection.At;
import org.spongepowered.asm.mixin.injection.Inject;
import org.spongepowered.asm.mixin.injection.callback.CallbackInfoReturnable;
import pl.m2x.commandsspy.CommandsSpy;

/**
 * Hook for MC 1.14.4 - 1.18.2: {@code execute} takes {@code ServerCommandSource}
 * directly and returns {@code int}. Compiled at {@code --release 8}, hence
 * instanceof + cast. See the wiki,
 * Version-Boundaries-And-Root-Causes -> "Why the boundaries sit where they do".
 */
@Mixin(CommandManager.class)
public class CommandManagerMixin {
    @SuppressWarnings({ "PMD.UnusedPrivateMethod", "PMD.UnusedFormalParameter" })
    @Inject(method = "execute", at = @At("HEAD"))
    private void onCommandExecute(ServerCommandSource source, String fullCommand, CallbackInfoReturnable<Integer> cir) {
        Entity entity = source.getEntity();

        if (entity instanceof ServerPlayerEntity) {
            ServerPlayerEntity player = (ServerPlayerEntity) entity;
            CommandsSpy.handleCommand(fullCommand, true, player.getName().getString());
        } else {
            CommandsSpy.handleCommand(fullCommand, false, source.getName());
        }
    }
}
