package pl.m2x.commandsspy;

import com.google.gson.Gson;
import com.google.gson.GsonBuilder;

import java.io.*;
import java.nio.file.Files;
import java.nio.file.Path;
import java.nio.file.Paths;
import java.util.List;
import java.util.ArrayList;

public class CommandsSpyConfig {
    /**
     * Exactly what FabricLoader.getConfigDir().resolve(...) resolved to before: a
     * dedicated server's gameDir IS its working directory. Spelled without a loader
     * API so the shared core compiles unchanged on Fabric, Quilt and Forge. This is
     * a server-side command logger; a client launched with an explicit --gameDir
     * elsewhere is out of scope, and regenerates defaults rather than failing.
     */
    private static final Path CONFIG_PATH = Paths.get("config", "commands-spy.json");
    private static final Gson GSON = new GsonBuilder().setPrettyPrinting().create();

    public List<String> blacklist = new ArrayList<>();
    public boolean logArguments = false;

    public static CommandsSpyConfig load() {
        CommandsSpyConfig config;

        if (Files.exists(CONFIG_PATH)) {
            try (Reader reader = Files.newBufferedReader(CONFIG_PATH)) {
                config = GSON.fromJson(reader, CommandsSpyConfig.class);
            } catch (IOException e) {
                throw new RuntimeException("Error reading config file", e);
            }
        } else {
            config = new CommandsSpyConfig();
            config.save();
        }

        return config;
    }

    public void save() {
        try {
            // FabricLoader.getConfigDir() created this; nothing else does.
            Files.createDirectories(CONFIG_PATH.getParent());
            try (Writer writer = Files.newBufferedWriter(CONFIG_PATH)) {
                GSON.toJson(this, writer);
            }
        } catch (IOException e) {
            throw new RuntimeException("Error writing config file", e);
        }
    }
}
