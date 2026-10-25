package org.punewatertracker.config;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.boot.CommandLineRunner;
import org.springframework.boot.autoconfigure.condition.ConditionalOnProperty;
import org.springframework.core.env.ConfigurableEnvironment;
import org.springframework.core.env.PropertySource;
import org.springframework.stereotype.Component;

/**
 * Diagnostic only -- logs Spring's REAL, resolved PropertySource precedence at startup, for
 * actual keys used in this project, so you can see the mechanism instead of just trusting a
 * description of it. Set breakpoints inside traceKey() and step through -- this walks the
 * exact same ordered list Spring's own Environment.getProperty() scans internally.
 *
 * Disabled by default (app.debug-properties=false) so it doesn't add startup noise in normal
 * runs -- same "opt-in, not a default" pattern as everything else in this project.
 */
@Component
@ConditionalOnProperty(name = "app.debug-properties", havingValue = "true")
public class PropertyPrecedenceDebugger implements CommandLineRunner {

    private static final Logger log = LoggerFactory.getLogger(PropertyPrecedenceDebugger.class);

    // Real keys from this project, chosen to show different resolution outcomes:
    // one that's almost always overridden by an env var in prod, one with a plain default,
    // one that's profile-specific, and one that should never be logged in full.
    private static final String[] KEYS_TO_TRACE = {
            "server.port",
            "spring.profiles.active",
            "app.ollama-host",
            "app.ollama-enabled",
            "app.admin-password",
    };

    private final ConfigurableEnvironment environment;

    public PropertyPrecedenceDebugger(ConfigurableEnvironment environment) {
        this.environment = environment;
    }

    @Override
    public void run(String... args) {
        log.info("=== PropertySource precedence (index 0 = highest priority) ===");
        int index = 0;
        for (PropertySource<?> source : environment.getPropertySources()) {
            log.info("  [{}] {} ({})", index++, source.getName(), source.getClass().getSimpleName());
        }

        log.info("=== Resolution trace for selected keys ===");
        for (String key : KEYS_TO_TRACE) {
            traceKey(key);
        }
    }

    /**
     * Walks the SAME ordered PropertySource list environment.getProperty() uses internally,
     * stopping at the first source that actually defines the key -- this loop IS how Spring
     * resolves precedence, not a simulation of it.
     */
    private void traceKey(String key) {
        for (PropertySource<?> source : environment.getPropertySources()) {
            if (source.containsProperty(key)) {
                Object rawValue = source.getProperty(key);
                String resolvedValue = environment.getProperty(key); // placeholders fully expanded
                boolean isSensitive = key.toLowerCase().contains("password") || key.toLowerCase().contains("secret");

                String displayValue = isSensitive ? "***REDACTED***" : resolvedValue;
                boolean wasPlaceholder = rawValue != null && !rawValue.toString().equals(resolvedValue);

                log.info("  {} = {}  <- won by source: \"{}\"{}",
                        key, displayValue, source.getName(),
                        wasPlaceholder ? " (raw value contained a ${...} placeholder, resolved above)" : "");
                return;
            }
        }
        log.info("  {} = <not set in any PropertySource>", key);
    }
}
