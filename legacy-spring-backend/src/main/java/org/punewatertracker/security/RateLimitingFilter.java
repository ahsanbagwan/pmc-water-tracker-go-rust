package org.punewatertracker.security;

import jakarta.servlet.FilterChain;
import jakarta.servlet.ServletException;
import jakarta.servlet.http.HttpServletRequest;
import jakarta.servlet.http.HttpServletResponse;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.core.Ordered;
import org.springframework.core.annotation.Order;
import org.springframework.data.redis.core.StringRedisTemplate;
import org.springframework.stereotype.Component;
import org.springframework.web.filter.OncePerRequestFilter;

import java.io.IOException;
import java.time.Duration;

/**
 * Rate limits login attempts by client IP, using Redis so the count is genuinely shared
 * across every pod -- the earlier in-memory ConcurrentHashMap version was correct on a single
 * Render instance, but silently wrong the moment multiple K8s replicas exist: an attacker
 * could simply alternate requests across pods and get N times the effective attempt budget,
 * since each pod tracked its own count with zero coordination.
 *
 * Uses Redis's INCR + EXPIRE -- INCR is atomic even under concurrent requests from different
 * pods hitting the same key simultaneously, which is exactly the property a shared counter
 * needs and a per-pod ConcurrentHashMap could never provide.
 *
 * FAILS OPEN if Redis itself is unreachable -- logged loudly, but a Redis outage should
 * degrade to "no rate limiting" rather than "nobody can log in at all," consistent with this
 * project's established resilience philosophy elsewhere (ReportEventPublisher,
 * OllamaReportAnalyzer): a secondary concern failing should never break the primary function.
 */
@Component
@Order(Ordered.HIGHEST_PRECEDENCE + 20)
public class RateLimitingFilter extends OncePerRequestFilter {

    private static final Logger log = LoggerFactory.getLogger(RateLimitingFilter.class);

    private static final String LOGIN_PATH = "/api/auth/login";
    private static final int MAX_ATTEMPTS_PER_WINDOW = 5;
    private static final Duration WINDOW = Duration.ofMinutes(1);

    private final StringRedisTemplate redisTemplate;

    public RateLimitingFilter(StringRedisTemplate redisTemplate) {
        this.redisTemplate = redisTemplate;
    }

    @Override
    protected void doFilterInternal(HttpServletRequest request, HttpServletResponse response,
                                     FilterChain filterChain) throws ServletException, IOException {
        boolean isLoginAttempt = "POST".equalsIgnoreCase(request.getMethod())
                && LOGIN_PATH.equals(request.getRequestURI());

        if (!isLoginAttempt) {
            filterChain.doFilter(request, response);
            return;
        }

        String clientIp = request.getRemoteAddr();
        if (isRateLimited(clientIp)) {
            log.warn("Rate limit exceeded for login attempts from {}", clientIp);
            response.setStatus(429); // Too Many Requests
            response.setContentType("application/json");
            response.getWriter().write("{\"error\":\"Too many login attempts. Try again in a minute.\"}");
            return;
        }

        filterChain.doFilter(request, response);
    }

    private boolean isRateLimited(String clientIp) {
        String key = "rate-limit:login:" + clientIp;
        try {
            // INCR creates the key at 1 if it doesn't exist yet, or atomically increments it
            // if it does -- this atomicity is exactly what makes it safe across concurrent
            // requests from different pods, unlike a read-then-write approach which would
            // have a race window.
            Long count = redisTemplate.opsForValue().increment(key);

            if (count != null && count == 1L) {
                // Only the request that just CREATED this key sets its expiry -- otherwise
                // every subsequent increment would reset the window, and it would never
                // actually expire under sustained attempts.
                redisTemplate.expire(key, WINDOW);
            }

            return count != null && count > MAX_ATTEMPTS_PER_WINDOW;
        } catch (Exception ex) {
            log.warn("Redis unreachable for rate limiting ({}) -- failing OPEN (allowing the "
                    + "request through) rather than blocking all logins during a Redis outage.",
                    ex.getMessage());
            return false;
        }
    }
}
