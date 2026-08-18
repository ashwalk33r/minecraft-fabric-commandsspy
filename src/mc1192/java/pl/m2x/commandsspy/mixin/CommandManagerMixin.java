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
 * Command-logging hook for Minecraft 1.19 - 1.20.2.
 *
 * <p>Identical to the 1.21.x hook except for the callback type. In this era
 * {@code CommandManager.execute} (intermediary {@code class_2170.method_9249})
 * is declared {@code (Lcom/mojang/brigadier/ParseResults;Ljava/lang/String;)I}
 * - verified against the mapped 1.20.1 artifact - it returns the number of
 * successful executions. Minecraft 1.20.3 flipped the return type to
 * {@code void} (e2e-proven: this jar boot-fails on 1.20.3/1.20.4), which is
 * why the range stops mid-minor at 1.20.2 and 1.20.3+ use the 1.21.x jar. Mixin
 * matches the injection target by name and then validates the descriptor, so a
 * {@code CallbackInfo} parameter here would fail at load time with
 * "CallbackInfoReturnable is required!", and the 1.21.x jar fails symmetrically
 * on these versions. That one parameter is the entire difference between the
 * jars.
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
            // Other sources (e.g., functions, data packs, signs, fullCommand blocks)
            CommandsSpy.handleCommand(fullCommand, false, source.getName());
        }
    }
}
