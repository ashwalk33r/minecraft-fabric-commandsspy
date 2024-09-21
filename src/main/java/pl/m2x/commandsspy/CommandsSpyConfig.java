package pl.m2x.commandsspy;

import com.google.gson.Gson;
import com.google.gson.GsonBuilder;
import net.fabricmc.loader.api.FabricLoader;

import java.io.*;
import java.nio.file.Files;
import java.nio.file.Path;
import java.util.List;
import java.util.ArrayList;

public class CommandsSpyConfig {
    private static final String CONFIG_FILE = "commands-spy.json";
    private static final Gson GSON = new GsonBuilder().setPrettyPrinting().create();

    public List<String> blacklist = new ArrayList<>();
    public boolean logArguments = false;

    public static CommandsSpyConfig load() {
        Path configPath = FabricLoader.getInstance().getConfigDir().resolve(CONFIG_FILE);
        CommandsSpyConfig config;

        if (Files.exists(configPath)) {
            try (Reader reader = Files.newBufferedReader(configPath)) {
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
        Path configPath = FabricLoader.getInstance().getConfigDir().resolve(CONFIG_FILE);

        try (Writer writer = Files.newBufferedWriter(configPath)) {
            GSON.toJson(this, writer);
        } catch (IOException e) {
            throw new RuntimeException("Error writing config file", e);
        }
    }
}
