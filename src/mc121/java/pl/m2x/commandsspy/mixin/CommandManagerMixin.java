package pl.m2x.commandsspy.mixin;

import com.mojang.brigadier.ParseResults;
import net.minecraft.server.command.CommandManager;
import net.minecraft.server.command.ServerCommandSource;
import net.minecraft.server.network.ServerPlayerEntity;
import net.minecraft.entity.Entity;
import org.spongepowered.asm.mixin.Mixin;
import org.spongepowered.asm.mixin.injection.At;
import org.spongepowered.asm.mixin.injection.Inject;
import org.spongepowered.asm.mixin.injection.callback.CallbackInfo;
import pl.m2x.commandsspy.CommandsSpy;

@Mixin(CommandManager.class)
public class CommandManagerMixin {
    @SuppressWarnings({ "PMD.UnusedPrivateMethod", "PMD.UnusedFormalParameter" })
    @Inject(method = "execute", at = @At("HEAD"))
    private void onCommandExecute(ParseResults<ServerCommandSource> parseResults, String fullCommand, CallbackInfo ci) {
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
