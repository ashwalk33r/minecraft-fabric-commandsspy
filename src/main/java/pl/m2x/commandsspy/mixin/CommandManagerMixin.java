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
    @Inject(method = "execute", at = @At("HEAD"))
    private void onCommandExecute(ParseResults<ServerCommandSource> parseResults, String command, CallbackInfo ci) {
        ServerCommandSource source = parseResults.getContext().getSource();
        Entity entity = source.getEntity();

        if (entity instanceof ServerPlayerEntity player) {
            String playerName = player.getName().getString();
            CommandsSpy.logCommand(command, "Player: " + playerName);
        } else {
            // Other sources (e.g., functions, data packs, signs, command blocks)
            CommandsSpy.logCommand(command, source.getName());
        }
    }
}