package org.punewatertracker.security;

import io.jsonwebtoken.Claims;
import io.jsonwebtoken.Jwts;
import jakarta.annotation.PostConstruct;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.core.env.Environment;
import org.springframework.stereotype.Service;

import java.security.KeyFactory;
import java.security.PrivateKey;
import java.security.PublicKey;
import java.security.spec.PKCS8EncodedKeySpec;
import java.security.spec.X509EncodedKeySpec;
import java.util.Base64;
import java.util.Date;
import java.util.function.Function;

@Service
public class JwtService {

    private final Environment environment;

    @Value("${app.jwt-private-key-b64:}")
    private String privateKeyBase64;

    @Value("${app.jwt-public-key-b64:}")
    private String publicKeyBase64;

    @Value("${app.jwt-expiration-ms:43200000}") // 12 hours
    private long jwtExpirationMs;

    private PrivateKey privateKey;
    private PublicKey publicKey;

    public JwtService(Environment environment) {
        this.environment = environment;
    }

    @PostConstruct
    public void init() throws Exception {
        if (environment.matchesProfiles("prod") && (privateKeyBase64.isBlank() || publicKeyBase64.isBlank())) {
            throw new IllegalStateException(
                    "Refusing to start: running with SPRING_PROFILES_ACTIVE=prod but "
                    + "JWT_PRIVATE_KEY_B64/JWT_PUBLIC_KEY_B64 are unset. Generate a real RSA "
                    + "keypair before deploying -- see JwtService's class comment for the "
                    + "openssl commands, or the project README.");
        }

        // Local dev with nothing configured: generate a throwaway in-memory keypair so the
        // app still starts and works, matching the "graceful when unconfigured" pattern used
        // elsewhere in this project -- tokens just won't survive a restart, which is fine for
        // local iteration.
        if (privateKeyBase64.isBlank() || publicKeyBase64.isBlank()) {
            var keyPair = Jwts.SIG.RS256.keyPair().build();
            this.privateKey = keyPair.getPrivate();
            this.publicKey = keyPair.getPublic();
            return;
        }

        KeyFactory keyFactory = KeyFactory.getInstance("RSA");
        this.privateKey = keyFactory.generatePrivate(
                new PKCS8EncodedKeySpec(Base64.getDecoder().decode(privateKeyBase64)));
        this.publicKey = keyFactory.generatePublic(
                new X509EncodedKeySpec(Base64.getDecoder().decode(publicKeyBase64)));
    }

    public String generateToken(String username, String role) {
        Date now = new Date();
        Date expiry = new Date(now.getTime() + jwtExpirationMs);
        return Jwts.builder()
                .subject(username)
                .claim("role", role)
                .issuedAt(now)
                .expiration(expiry)
                .signWith(privateKey, Jwts.SIG.RS256)
                .compact();
    }

    public String extractUsername(String token) {
        return extractClaim(token, Claims::getSubject);
    }

    public String extractRole(String token) {
        return extractAllClaims(token).get("role", String.class);
    }

    public boolean isTokenValid(String token, String expectedUsername) {
        String username = extractUsername(token);
        return username.equals(expectedUsername) && !isExpired(token);
    }

    private boolean isExpired(String token) {
        return extractClaim(token, Claims::getExpiration).before(new Date());
    }

    private <T> T extractClaim(String token, Function<Claims, T> resolver) {
        return resolver.apply(extractAllClaims(token));
    }

    private Claims extractAllClaims(String token) {
        return Jwts.parser()
                .verifyWith(publicKey)
                .build()
                .parseSignedClaims(token)
                .getPayload();
    }
}
