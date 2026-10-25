package org.punewatertracker.security;

import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.punewatertracker.model.Role;
import org.punewatertracker.repository.UserRepository;
import org.springframework.core.env.Environment;
import org.springframework.security.crypto.password.PasswordEncoder;
import org.springframework.test.util.ReflectionTestUtils;

import static org.assertj.core.api.Assertions.assertThatThrownBy;
import static org.mockito.ArgumentMatchers.any;
import static org.mockito.ArgumentMatchers.anyString;
import static org.mockito.Mockito.argThat;
import static org.mockito.Mockito.mock;
import static org.mockito.Mockito.never;
import static org.mockito.Mockito.verify;
import static org.mockito.Mockito.verifyNoInteractions;
import static org.mockito.Mockito.when;

class AdminBootstrapTest {

    private UserRepository userRepository;
    private PasswordEncoder passwordEncoder;

    @BeforeEach
    void setUp() {
        userRepository = mock(UserRepository.class);
        passwordEncoder = mock(PasswordEncoder.class);
        when(passwordEncoder.encode(anyString())).thenReturn("hashed");
    }

    @Test
    void run_prodProfileWithDefaultPassword_refusesToStart() {
        Environment prodEnv = mock(Environment.class);
        when(prodEnv.matchesProfiles("prod")).thenReturn(true);

        AdminBootstrap bootstrap = new AdminBootstrap(userRepository, passwordEncoder, prodEnv);
        ReflectionTestUtils.setField(bootstrap, "adminPassword", "change-me-now");

        assertThatThrownBy(bootstrap::run)
                .isInstanceOf(IllegalStateException.class)
                .hasMessageContaining("ADMIN_PASSWORD");

        verifyNoInteractions(userRepository);
    }

    @Test
    void run_devProfileWithDefaultPassword_stillBootstraps() {
        Environment devEnv = mock(Environment.class);
        when(devEnv.matchesProfiles("prod")).thenReturn(false);
        when(userRepository.count()).thenReturn(0L);

        AdminBootstrap bootstrap = new AdminBootstrap(userRepository, passwordEncoder, devEnv);
        ReflectionTestUtils.setField(bootstrap, "adminUsername", "admin");
        ReflectionTestUtils.setField(bootstrap, "adminPassword", "change-me-now");

        bootstrap.run();

        verify(userRepository).save(argThat(user ->
                user.getUsername().equals("admin") && user.getRole() == Role.ADMIN));
    }

    @Test
    void run_prodProfileWithRealPassword_bootstrapsNormally() {
        Environment prodEnv = mock(Environment.class);
        when(prodEnv.matchesProfiles("prod")).thenReturn(true);
        when(userRepository.count()).thenReturn(0L);

        AdminBootstrap bootstrap = new AdminBootstrap(userRepository, passwordEncoder, prodEnv);
        ReflectionTestUtils.setField(bootstrap, "adminUsername", "admin");
        ReflectionTestUtils.setField(bootstrap, "adminPassword", "a-real-strong-password");

        bootstrap.run();

        verify(userRepository).save(any());
    }

    @Test
    void run_usersAlreadyExist_doesNothing() {
        Environment devEnv = mock(Environment.class);
        when(devEnv.matchesProfiles("prod")).thenReturn(false);
        when(userRepository.count()).thenReturn(1L);

        AdminBootstrap bootstrap = new AdminBootstrap(userRepository, passwordEncoder, devEnv);
        ReflectionTestUtils.setField(bootstrap, "adminPassword", "change-me-now");

        bootstrap.run();

        verify(userRepository, never()).save(any());
    }
}
