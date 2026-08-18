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
 * Command-logging hook for Minecraft 1.14.4 - 1.18.2.
 *
 * <p>In this era {@code CommandManager.execute} (intermediary
 * {@code class_2170.method_9249}) is declared
 * {@code (Lclass_2168;Ljava/lang/String;)I} - verified against the mapped
 * 1.16.5 artifact: the {@code ServerCommandSource} arrives DIRECTLY as the
 * first parameter (no {@code ParseResults} wrapper, so no {@code getContext()}
 * hop) and the method returns {@code int}, so the callback must be
 * {@link CallbackInfoReturnable}{@code <Integer>}, not {@code CallbackInfo}.
 * 1.19 wrapped the source in {@code ParseResults} (still returning
 * {@code int}) - see {@code src/mc1192} - and 1.20.3 additionally flipped the
 * return to {@code void} - see {@code src/mc121}.
 *
 * <p>This source set compiles at {@code --release 8} (the era's own JVM
 * floor), so the player check is a classic instanceof + cast: pattern
 * matching is Java 16+ syntax and would not compile here.
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
            // Other sources (e.g., server console, RCON, functions, command blocks)
            CommandsSpy.handleCommand(fullCommand, false, source.getName());
        }
    }
}
