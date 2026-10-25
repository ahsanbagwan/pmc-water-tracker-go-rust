package org.punewatertracker.config;

import org.junit.jupiter.api.Test;
import org.springframework.core.env.MapPropertySource;
import org.springframework.core.env.MutablePropertySources;
import org.springframework.core.env.StandardEnvironment;

import java.util.Map;

import static org.assertj.core.api.Assertions.assertThat;

/**
 * These tests use Spring's REAL StandardEnvironment/MutablePropertySources classes -- the same
 * ones Spring Boot builds internally at startup -- to prove the precedence rules explained
 * elsewhere in this project, rather than just asserting them from memory.
 *
 * Key mechanic: MutablePropertySources' iteration order IS precedence order. Whichever source
 * comes first when iterated wins on a conflicting key. addFirst() inserts at highest priority;
 * addLast() inserts at lowest. This mirrors how Spring Boot actually layers env vars above
 * profile-specific files above the base application.properties.
 */
class PropertyPrecedenceTest {

    @Test
    void higherPriorityPropertySource_winsOnConflictingKey() {
        StandardEnvironment environment = new StandardEnvironment();
        MutablePropertySources sources = environment.getPropertySources();

        // Simulates: application.properties has a default, but an OS env var overrides it --
        // exactly what happens with app.ollama-host=${OLLAMA_HOST:http://localhost:11434}
        // once a real OLLAMA_HOST env var is set on the deployed instance.
        sources.addLast(new MapPropertySource("simulated-application.properties",
                Map.of("app.ollama-host", "http://localhost:11434")));
        sources.addFirst(new MapPropertySource("simulated-OS-environment",
                Map.of("app.ollama-host", "http://oracle-vm.example.com:11434")));

        assertThat(environment.getProperty("app.ollama-host"))
                .as("The higher-priority source (simulated env var) should win over the "
                        + "lower-priority file default")
                .isEqualTo("http://oracle-vm.example.com:11434");
    }

    @Test
    void withNoOverride_fallsBackToLowerPrioritySource() {
        StandardEnvironment environment = new StandardEnvironment();
        MutablePropertySources sources = environment.getPropertySources();

        // No env var this time -- only the base file's default exists at all.
        sources.addLast(new MapPropertySource("simulated-application.properties",
                Map.of("app.ollama-host", "http://localhost:11434")));

        assertThat(environment.getProperty("app.ollama-host"))
                .as("With nothing overriding it, the only source that defines the key wins by default")
                .isEqualTo("http://localhost:11434");
    }

    @Test
    void secondActiveProfile_winsOverFirstOnOverlappingKey() {
        StandardEnvironment environment = new StandardEnvironment();
        MutablePropertySources sources = environment.getPropertySources();

        // Simulates spring.profiles.active=dev,chaos-monkey: Spring Boot adds the LAST-listed
        // profile's PropertySource at higher priority than earlier-listed ones. This is why,
        // in the actual chaos-monkey testing session, "dev,chaos-monkey" (that exact order)
        // mattered -- if these two files ever defined the same key, chaos-monkey (listed
        // second) is the one that would win, not dev.
        sources.addLast(new MapPropertySource("application-dev.properties",
                Map.of("chaos.monkey.assaults.level", "5")));
        sources.addFirst(new MapPropertySource("application-chaos-monkey.properties",
                Map.of("chaos.monkey.assaults.level", "1")));

        assertThat(environment.getProperty("chaos.monkey.assaults.level"))
                .as("The second-listed active profile (chaos-monkey) should win on an overlapping key")
                .isEqualTo("1");
    }

    @Test
    void placeholderSyntax_resolvesAgainstTheSameMergedStack() {
        StandardEnvironment environment = new StandardEnvironment();
        MutablePropertySources sources = environment.getPropertySources();

        // Simulates the actual pattern used throughout this project's application.properties:
        // app.ollama-host=${OLLAMA_HOST:http://localhost:11434}
        // The ${...:default} syntax isn't special-cased separately -- it's resolved by looking
        // OLLAMA_HOST up in this SAME merged PropertySource stack, with the literal after the
        // colon used only if OLLAMA_HOST isn't found anywhere in it.
        sources.addLast(new MapPropertySource("simulated-application.properties",
                Map.of("app.ollama-host", "${OLLAMA_HOST:http://localhost:11434}")));

        // Case 1: no OLLAMA_HOST anywhere -- falls back to the literal default.
        assertThat(environment.getProperty("app.ollama-host")).isEqualTo("http://localhost:11434");

        // Case 2: add a higher-priority source defining OLLAMA_HOST -- the placeholder should
        // now resolve to that value instead of the literal default.
        sources.addFirst(new MapPropertySource("simulated-OS-environment",
                Map.of("OLLAMA_HOST", "http://oracle-vm.example.com:11434")));

        assertThat(environment.getProperty("app.ollama-host"))
                .as("The ${OLLAMA_HOST:...} placeholder should resolve against the env var, "
                        + "not just use its own file's literal default")
                .isEqualTo("http://oracle-vm.example.com:11434");
    }
}
