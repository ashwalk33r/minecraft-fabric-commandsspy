package pl.m2x.commandsspy.mixin;

import com.mojang.brigadier.ParseResults;
import net.minecraft.server.command.CommandManager;
import net.minecraft.server.command.ServerCommandSource;
import net.minecraft.server.network.ServerPlayerEntity;
import net.minecraft.entity.Entity;
import org.spongepowered.asm.mixin.Mixin;
import org.spongepowered.asm.mixin.injection.At;
import org.spongepowered.asm.mixin.injection.Inject;
import org.spongepowered.asm.mixin.injection.callback.CallbackInfoReturnable;
import pl.m2x.commandsspy.CommandsSpy;

/**
 * Hook for MC 1.19 - 1.20.2: {@code execute} takes {@code ParseResults} and
 * returns {@code int}, so the callback is {@code CallbackInfoReturnable<Integer>}.
 * 1.20.3+ returns {@code void} - see src/mc121 and docs/version-matrix.md.
 */
@Mixin(CommandManager.class)
public class CommandManagerMixin {
    @SuppressWarnings({ "PMD.UnusedPrivateMethod", "PMD.UnusedFormalParameter" })
    @Inject(method = "execute", at = @At("HEAD"))
    private void onCommandExecute(ParseResults<ServerCommandSource> parseResults, String fullCommand, CallbackInfoReturnable<Integer> cir) {
        ServerCommandSource source = parseResults.getContext().getSource();
        Entity entity = source.getEntity();

        if (entity instanceof ServerPlayerEntity player) {
            CommandsSpy.handleCommand(fullCommand, true, player.getName().getString());
        } else {
            CommandsSpy.handleCommand(fullCommand, false, source.getName());
        }
    }
}
