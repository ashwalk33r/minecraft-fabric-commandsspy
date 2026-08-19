package pl.m2x.commandsspy.mixin;

import com.mojang.brigadier.ParseResults;
import net.minecraft.commands.CommandSourceStack;
import net.minecraft.commands.Commands;
import net.minecraft.server.level.ServerPlayer;
import net.minecraft.world.entity.Entity;
import org.spongepowered.asm.mixin.Mixin;
import org.spongepowered.asm.mixin.injection.At;
import org.spongepowered.asm.mixin.injection.Inject;
import org.spongepowered.asm.mixin.injection.callback.CallbackInfo;
import pl.m2x.commandsspy.CommandsSpy;

/**
 * Hook for MC 26.x: the runtime is unobfuscated, so this targets official
 * Mojang names ({@code Commands.performCommand}). See docs/version-matrix.md.
 */
@Mixin(Commands.class)
public class CommandManagerMixin {
    @SuppressWarnings({ "PMD.UnusedPrivateMethod", "PMD.UnusedFormalParameter" })
    @Inject(method = "performCommand", at = @At("HEAD"))
    private void onCommandExecute(ParseResults<CommandSourceStack> parseResults, String fullCommand, CallbackInfo ci) {
        CommandSourceStack source = parseResults.getContext().getSource();
        Entity entity = source.getEntity();

        if (entity instanceof ServerPlayer player) {
            CommandsSpy.handleCommand(fullCommand, true, player.getName().getString());
        } else {
            CommandsSpy.handleCommand(fullCommand, false, source.getTextName());
        }
    }
}
