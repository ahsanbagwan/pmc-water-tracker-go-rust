package org.punewatertracker.web;

import jakarta.servlet.http.HttpServletRequest;
import jakarta.servlet.http.HttpServletResponse;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.web.method.HandlerMethod;
import org.springframework.web.servlet.HandlerInterceptor;

/**
 * A genuine Spring MVC HandlerInterceptor -- a fundamentally different mechanism from a
 * Servlet Filter (see CorrelationIdFilter/RequestLoggingFilter/RateLimitingFilter above).
 * Interceptors run INSIDE Spring MVC's own dispatch, AFTER the framework has already resolved
 * which specific controller method will handle the request. A Filter runs before Spring MVC is
 * even involved, so it fundamentally cannot see this -- only an interceptor can.
 *
 * Concretely demonstrates that capability: logs the actual resolved controller class + method
 * name, and times ONLY the handler's own execution -- excluding filter-chain overhead (the
 * three filters above, Spring Security's checks, etc.) -- a genuinely different, narrower
 * measurement than RequestLoggingFilter's total-request timing.
 */
public class HandlerTimingInterceptor implements HandlerInterceptor {

    private static final Logger log = LoggerFactory.getLogger(HandlerTimingInterceptor.class);
    private static final String START_TIME_ATTRIBUTE = "handlerStartTime";

    @Override
    public boolean preHandle(HttpServletRequest request, HttpServletResponse response, Object handler) {
        request.setAttribute(START_TIME_ATTRIBUTE, System.currentTimeMillis());
        return true; // never blocks a request -- purely observational
    }

    @Override
    public void afterCompletion(HttpServletRequest request, HttpServletResponse response,
                                 Object handler, Exception ex) {
        Long startTime = (Long) request.getAttribute(START_TIME_ATTRIBUTE);
        if (startTime == null) {
            return;
        }
        long handlerDurationMs = System.currentTimeMillis() - startTime;

        String handlerDescription = (handler instanceof HandlerMethod handlerMethod)
                ? handlerMethod.getBeanType().getSimpleName() + "." + handlerMethod.getMethod().getName()
                : handler.toString();

        log.debug("Handler {} took {}ms (excludes filter-chain overhead)", handlerDescription, handlerDurationMs);
    }
}
