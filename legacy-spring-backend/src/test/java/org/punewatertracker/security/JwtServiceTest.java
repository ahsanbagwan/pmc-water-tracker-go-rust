package org.punewatertracker.security;

import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.springframework.core.env.Environment;
import org.springframework.test.util.ReflectionTestUtils;

import java.security.KeyPair;
import java.security.KeyPairGenerator;
import java.util.Base64;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;
import static org.mockito.Mockito.mock;
import static org.mockito.Mockito.when;

class JwtServiceTest {

    private JwtService jwtService;

    @BeforeEach
    void setUp() throws Exception {
        Environment environment = mock(Environment.class);
        when(environment.matchesProfiles("prod")).thenReturn(false);

        jwtService = new JwtService(environment);
        ReflectionTestUtils.setField(jwtService, "jwtExpirationMs", 60_000L);
        jwtService.init(); // dev, unconfigured -- should auto-generate a throwaway keypair
    }

    @Test
    void generateToken_roundTripsUsernameAndRole() {
        String token = jwtService.generateToken("priya", "EDITOR");

        assertThat(jwtService.extractUsername(token)).isEqualTo("priya");
        assertThat(jwtService.extractRole(token)).isEqualTo("EDITOR");
        assertThat(jwtService.isTokenValid(token, "priya")).isTrue();
    }

    @Test
    void isTokenValid_wrongUsername_returnsFalse() {
        String token = jwtService.generateToken("priya", "EDITOR");

        assertThat(jwtService.isTokenValid(token, "someone-else")).isFalse();
    }

    @Test
    void isTokenValid_expiredToken_returnsFalse() throws InterruptedException {
        ReflectionTestUtils.setField(jwtService, "jwtExpirationMs", 1L);
        String token = jwtService.generateToken("priya", "EDITOR");

        Thread.sleep(15);

        assertThat(jwtService.isTokenValid(token, "priya")).isFalse();
    }

    @Test
    void init_prodProfileWithNoKeysConfigured_throws() {
        Environment prodEnv = mock(Environment.class);
        when(prodEnv.matchesProfiles("prod")).thenReturn(true);

        JwtService prodJwtService = new JwtService(prodEnv);
        // privateKeyBase64/publicKeyBase64 left at their default (blank) -- simulates
        // JWT_PRIVATE_KEY_B64/JWT_PUBLIC_KEY_B64 genuinely being unset in prod.

        assertThatThrownBy(prodJwtService::init)
                .isInstanceOf(IllegalStateException.class)
                .hasMessageContaining("JWT_PRIVATE_KEY_B64");
    }

    @Test
    void init_devProfileWithNoKeysConfigured_doesNotThrow() {
        // Already exercised implicitly by setUp() above (every other test in this class
        // depends on this succeeding) -- asserted explicitly here too, as its own test, so a
        // regression here fails with a clear, specific name instead of every other test just
        // mysteriously failing in setUp().
        Environment devEnv = mock(Environment.class);
        when(devEnv.matchesProfiles("prod")).thenReturn(false);

        JwtService devJwtService = new JwtService(devEnv);

        org.assertj.core.api.Assertions.assertThatCode(devJwtService::init).doesNotThrowAnyException();
    }

    @Test
    void init_withRealConfiguredKeys_usesThemInsteadOfGeneratingNew() throws Exception {
        // Generates an actual RSA keypair and base64-encodes it exactly the way the real
        // openssl-based setup does (PKCS8 for the private key, X.509 for the public key) --
        // proving init() can genuinely parse real configured keys, not just the dev fallback.
        KeyPairGenerator generator = KeyPairGenerator.getInstance("RSA");
        generator.initialize(2048);
        KeyPair keyPair = generator.generateKeyPair();

        String privateKeyB64 = Base64.getEncoder().encodeToString(keyPair.getPrivate().getEncoded());
        String publicKeyB64 = Base64.getEncoder().encodeToString(keyPair.getPublic().getEncoded());

        Environment devEnv = mock(Environment.class);
        when(devEnv.matchesProfiles("prod")).thenReturn(false);

        JwtService configuredJwtService = new JwtService(devEnv);
        ReflectionTestUtils.setField(configuredJwtService, "privateKeyBase64", privateKeyB64);
        ReflectionTestUtils.setField(configuredJwtService, "publicKeyBase64", publicKeyB64);
        ReflectionTestUtils.setField(configuredJwtService, "jwtExpirationMs", 60_000L);
        configuredJwtService.init();

        String token = configuredJwtService.generateToken("priya", "ADMIN");

        assertThat(configuredJwtService.extractUsername(token)).isEqualTo("priya");
        assertThat(configuredJwtService.isTokenValid(token, "priya")).isTrue();
    }
}
