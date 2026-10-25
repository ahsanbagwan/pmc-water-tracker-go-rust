package org.punewatertracker.web;

import jakarta.servlet.FilterChain;
import jakarta.servlet.ServletException;
import jakarta.servlet.http.HttpServletRequest;
import jakarta.servlet.http.HttpServletResponse;
import org.slf4j.MDC;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.core.Ordered;
import org.springframework.core.annotation.Order;
import org.springframework.stereotype.Component;
import org.springframework.web.filter.OncePerRequestFilter;

import java.io.IOException;
import java.util.Arrays;
import java.util.List;

/**
 * Extracts a CONFIGURABLE allowlist of HTTP headers into MDC, so they become separate,
 * filterable fields once shipped -- LogstashEncoder includes every MDC entry as a top-level
 * JSON field by default; Loki4j can do the same via label mapping.
 *
 * WHY A FILTER, NOT AN INTERCEPTOR: a HandlerInterceptor only starts running AFTER Spring MVC
 * has already resolved a controller method -- meaning JwtAuthFilter and RateLimitingFilter
 * would have already run (and possibly already logged something) before an interceptor's
 * preHandle() ever fires. A Filter, at the very front of the chain, has no such blind spot.
 * Ordered right after CorrelationIdFilter (HIGHEST_PRECEDENCE) and before RequestLoggingFilter
 * (HIGHEST_PRECEDENCE + 10), so its own summary log line gets these fields too.
 *
 * Deliberately an ALLOWLIST, not "log every header": headers can carry sensitive data
 * (Authorization, Cookie) that should never be duplicated into log storage. Unset by
 * default (app.mdc-headers empty) -- same opt-in pattern as everything else in this project.
 */
@Component
@Order(Ordered.HIGHEST_PRECEDENCE + 1)
public class HeaderMdcFilter extends OncePerRequestFilter {

    private final List<String> headerNames;

    public HeaderMdcFilter(@Value("${app.mdc-headers:}") String configuredHeaders) {
        this.headerNames = configuredHeaders.isBlank()
                ? List.of()
                : Arrays.stream(configuredHeaders.split(","))
                        .map(String::trim)
                        .filter(h -> !h.isEmpty())
                        .toList();
    }

    @Override
    protected void doFilterInternal(HttpServletRequest request, HttpServletResponse response,
                                     FilterChain filterChain) throws ServletException, IOException {
        List<String> addedKeys = headerNames.stream()
                .filter(name -> request.getHeader(name) != null)
                .toList();

        addedKeys.forEach(name -> MDC.put(mdcKeyFor(name), request.getHeader(name)));

        try {
            filterChain.doFilter(request, response);
        } finally {
            // Only remove what THIS filter added -- never blindly clear all of MDC here,
            // since CorrelationIdFilter's own entry (set before this filter runs, in the
            // wrapping call) still needs to survive until ITS OWN finally block runs, further
            // up the call stack, after this one returns.
            addedKeys.forEach(name -> MDC.remove(mdcKeyFor(name)));
        }
    }

    /** Normalizes a header name into a clean field key, e.g. "X-Tenant-Id" -> "header.x-tenant-id" */
    private String mdcKeyFor(String headerName) {
        return "header." + headerName.toLowerCase();
    }
}
