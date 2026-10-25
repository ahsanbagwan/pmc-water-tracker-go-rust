package org.punewatertracker.security;

import org.punewatertracker.model.Role;
import org.punewatertracker.model.User;
import org.punewatertracker.repository.UserRepository;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.boot.CommandLineRunner;
import org.springframework.core.env.Environment;
import org.springframework.security.crypto.password.PasswordEncoder;
import org.springframework.stereotype.Component;

@Component
public class AdminBootstrap implements CommandLineRunner {

    private static final Logger log = LoggerFactory.getLogger(AdminBootstrap.class);
    private static final String DEFAULT_PASSWORD = "change-me-now";

    private final UserRepository userRepository;
    private final PasswordEncoder passwordEncoder;
    private final Environment environment;

    @Value("${app.admin-username:admin}")
    private String adminUsername;

    @Value("${app.admin-password:change-me-now}")
    private String adminPassword;

    public AdminBootstrap(UserRepository userRepository, PasswordEncoder passwordEncoder, Environment environment) {
        this.userRepository = userRepository;
        this.passwordEncoder = passwordEncoder;
        this.environment = environment;
    }

    @Override
    public void run(String... args) {
        if (environment.matchesProfiles("prod") && DEFAULT_PASSWORD.equals(adminPassword)) {
            throw new IllegalStateException(
                    "Refusing to start: running with SPRING_PROFILES_ACTIVE=prod but ADMIN_PASSWORD "
                    + "is unset (still the dev default). Set a real ADMIN_PASSWORD env var before deploying.");
        }

        if (userRepository.count() > 0) {
            return; // already bootstrapped
        }
        User admin = new User(adminUsername, passwordEncoder.encode(adminPassword), Role.ADMIN);
        userRepository.save(admin);
        log.info("Created initial admin user '{}'. Set ADMIN_USERNAME/ADMIN_PASSWORD env vars "
                + "in production instead of using defaults.", adminUsername);
    }
}
