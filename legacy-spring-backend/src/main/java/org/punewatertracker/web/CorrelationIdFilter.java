package org.punewatertracker.web;

import jakarta.servlet.FilterChain;
import jakarta.servlet.ServletException;
import jakarta.servlet.http.HttpServletRequest;
import jakarta.servlet.http.HttpServletResponse;
import org.slf4j.MDC;
import org.springframework.core.Ordered;
import org.springframework.core.annotation.Order;
import org.springframework.stereotype.Component;
import org.springframework.web.filter.OncePerRequestFilter;

import java.io.IOException;
import java.util.UUID;

/**
 * Assigns a correlation ID to every request -- reused from an incoming X-Correlation-Id header
 * if a caller/load balancer already set one, otherwise freshly generated -- and puts it in
 * SLF4J's MDC so EVERY log line emitted while handling this request automatically includes it.
 * Genuinely valuable now that logs ship to Loki: without this, correlating multiple log lines
 * from the same request (especially under concurrent traffic, where Tomcat's threads
 * interleave freely) is close to impossible.
 *
 * Registered at the highest possible precedence (Ordered.HIGHEST_PRECEDENCE) so the
 * correlation ID is available even for log lines emitted by Spring Security's own filters,
 * which run very early in the chain.
 *
 * MDC is thread-local, and Tomcat REUSES worker threads across requests -- forgetting to clean
 * up in a finally block would leak this request's correlation ID into whatever the same
 * thread handles next. This is a classic, easy-to-miss bug with MDC-based logging.
 */
@Component
@Order(Ordered.HIGHEST_PRECEDENCE)
public class CorrelationIdFilter extends OncePerRequestFilter {

    public static final String CORRELATION_ID_HEADER = "X-Correlation-Id";
    public static final String MDC_KEY = "correlationId";

    @Override
    protected void doFilterInternal(HttpServletRequest request, HttpServletResponse response,
                                     FilterChain filterChain) throws ServletException, IOException {
        String correlationId = request.getHeader(CORRELATION_ID_HEADER);
        if (correlationId == null || correlationId.isBlank()) {
            correlationId = UUID.randomUUID().toString();
        }

        MDC.put(MDC_KEY, correlationId);
        response.setHeader(CORRELATION_ID_HEADER, correlationId);

        try {
            filterChain.doFilter(request, response);
        } finally {
            MDC.remove(MDC_KEY);
        }
    }
}
