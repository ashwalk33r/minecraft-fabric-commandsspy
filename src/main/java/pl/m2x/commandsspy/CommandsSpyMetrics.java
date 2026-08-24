package pl.m2x.commandsspy;

import com.google.gson.Gson;
import com.google.gson.GsonBuilder;
import pl.m2x.commandsspy.bstats.MetricsBase;
import pl.m2x.commandsspy.bstats.charts.SimplePie;
import pl.m2x.commandsspy.bstats.json.JsonObjectBuilder;

import java.io.IOException;
import java.io.Reader;
import java.io.Writer;
import java.nio.file.Files;
import java.nio.file.Path;
import java.nio.file.Paths;
import java.util.UUID;

/**
 * bStats reporting for every loader this mod ships on.
 *
 * <p>bStats publishes no Fabric/Forge/NeoForge platform, and the Bukkit {@code Metrics.java}
 * it hands out imports {@code org.bukkit.*}, so it cannot compile here. This class is the
 * platform half of the standard split: {@code pl.m2x.commandsspy.bstats.MetricsBase} is the
 * vendored, unmodified engine, and this file is what every official
 * {@code org/bstats/<platform>/Metrics.java} is - the part that says what to send. The
 * service is registered under Bukkit (id 33622), which is why the platform string is
 * {@code "bukkit"} even though no supported loader is Bukkit.
 *
 * <p>ponytail: this fires on integrated clients too, not only dedicated servers, so a
 * handful of single-player instances count as "servers". Gating would cost a per-loader
 * environment check across seven entrypoints for a mod that is server-side in every
 * practical sense; add it if the numbers ever look inflated.
 */
public final class CommandsSpyMetrics {

	private static final int SERVICE_ID = 33622;

	/**
	 * Deliberately not inside {@code commands-spy.json}: a generated server UUID written
	 * back there would break CommandsSpyConfigTest's "load() does not rewrite the file"
	 * assertion, and bStats' own convention is a separate file anyway
	 * ({@code plugins/bStats/config.yml} on Bukkit). CWD-relative for the same reason
	 * {@link CommandsSpyConfig} is - no loader API in the shared core.
	 */
	static final Path CONFIG_PATH = Paths.get("config", "bStats", "config.json");

	private static final Gson GSON = new GsonBuilder().setPrettyPrinting().create();

	private final String loader;
	private final String modVersion;
	private final String mcVersion;

	private CommandsSpyMetrics(final String loader, final String modVersion, final String mcVersion) {
		this.loader = loader;
		this.modVersion = modVersion;
		this.mcVersion = mcVersion;
	}

	/** The on-disk opt-out. Public fields, same shape as {@link CommandsSpyConfig}. */
	static class Config {
		boolean enabled = true;
		String serverUuid;
	}

	/**
	 * Called by {@link CommandsSpy#startMetrics}, never directly by a loader.
	 *
	 * @param loader loader name as it should appear on the bStats "loader" chart
	 * @param modVersion this mod's version, for the Plugin Version chart
	 * @param mcVersion the Minecraft version the loader reports
	 */
	static void start(final String loader, final String modVersion, final String mcVersion) {
		if (!enabledByEnvironment()) {
			CommandsSpy.LOGGER.info("[CommandsSpy] bStats disabled by BSTATS_ENABLED/bstats.enabled.");
			return;
		}
		final Config config = loadConfig();
		if (!config.enabled) {
			return;
		}
		new CommandsSpyMetrics(loader, modVersion, mcVersion).submit(config.serverUuid);
	}

	/**
	 * The environment kill switch, checked before the config file so the e2e containers
	 * stay offline without seeding a file in both of their legs.
	 *
	 * @return false when either {@code -Dbstats.enabled=false} or {@code BSTATS_ENABLED=false}
	 */
	static boolean enabledByEnvironment() {
		return !"false".equalsIgnoreCase(System.getProperty("bstats.enabled"))
				&& !"false".equalsIgnoreCase(System.getenv("BSTATS_ENABLED"));
	}

	/**
	 * Reads {@code config/bStats/config.json}, creating it with a fresh server UUID when it
	 * is absent or unusable.
	 *
	 * @return the config, never null, always with a serverUuid
	 */
	static Config loadConfig() {
		if (Files.exists(CONFIG_PATH)) {
			try (Reader reader = Files.newBufferedReader(CONFIG_PATH)) {
				final Config config = GSON.fromJson(reader, Config.class);
				if (config != null && config.serverUuid != null) {
					return config;
				}
			} catch (IOException e) {
				CommandsSpy.LOGGER.warn("[CommandsSpy] Could not read {}; regenerating.", CONFIG_PATH, e);
			}
		}
		final Config config = new Config();
		config.serverUuid = UUID.randomUUID().toString();
		save(config);
		return config;
	}

	private static void save(final Config config) {
		try {
			Files.createDirectories(CONFIG_PATH.getParent());
			try (Writer writer = Files.newBufferedWriter(CONFIG_PATH)) {
				GSON.toJson(config, writer);
			}
		} catch (IOException e) {
			// Not fatal: an unwritable config dir must not cost the server its command log.
			CommandsSpy.LOGGER.warn("[CommandsSpy] Could not write {}.", CONFIG_PATH, e);
		}
	}

	private void submit(final String serverUuid) {
		// submitTaskConsumer is null on purpose: bStats then submits on its own daemon
		// thread ("bStats-Metrics"), so no main-thread scheduler is needed and server
		// shutdown is unaffected - which is also why no shutdown() hook is registered.
		final MetricsBase base = new MetricsBase(
				"bukkit", serverUuid, SERVICE_ID, true,
				this::appendPlatformData, this::appendServiceData, null, () -> true,
				(message, error) -> CommandsSpy.LOGGER.warn(message, error),
				CommandsSpy.LOGGER::info,
				false, false, false, false);
		// The loader also lands in bukkitName, but the Bukkit backend parses bukkitVersion
		// expecting Paper/Spigot-shaped strings, so these two pies are the reliable read.
		base.addCustomChart(new SimplePie("loader", () -> loader));
		base.addCustomChart(new SimplePie("minecraft_version", () -> mcVersion));
	}

	private void appendPlatformData(final JsonObjectBuilder builder) {
		// playerAmount and onlineMode are omitted, not zeroed: the shared core has no
		// Minecraft API access across four mapping eras, and a 0 would populate the
		// Players chart with a lie rather than leave it empty.
		appendIfKnown(builder, "bukkitName", loader);
		appendIfKnown(builder, "bukkitVersion", mcVersion);
		appendIfKnown(builder, "javaVersion", System.getProperty("java.version"));
		appendIfKnown(builder, "osName", System.getProperty("os.name"));
		appendIfKnown(builder, "osArch", System.getProperty("os.arch"));
		appendIfKnown(builder, "osVersion", System.getProperty("os.version"));
		builder.appendField("coreCount", Runtime.getRuntime().availableProcessors());
	}

	private void appendServiceData(final JsonObjectBuilder builder) {
		appendIfKnown(builder, "pluginVersion", modVersion);
	}

	/** Anything unknown is skipped rather than sent as "unknown". */
	private static void appendIfKnown(final JsonObjectBuilder builder, final String key, final String value) {
		if (value != null && !value.trim().isEmpty()) {
			builder.appendField(key, value);
		}
	}
}
