package org.punewatertracker.audit;

import org.aspectj.lang.ProceedingJoinPoint;
import org.aspectj.lang.annotation.Around;
import org.aspectj.lang.annotation.Aspect;
import org.punewatertracker.model.Locality;
import org.springframework.security.core.Authentication;
import org.springframework.security.core.context.SecurityContextHolder;
import org.springframework.stereotype.Component;

import java.time.Instant;
import java.util.Arrays;
import java.util.stream.Collectors;

/**
 * Wraps every @Audited method: records who called it (from the security context), what it was
 * called with, whether it succeeded, and any error -- then lets the original call proceed or
 * fail exactly as it would have without this aspect. The audited method never needs to know
 * it's being logged; adding @Audited to a new method is the entire integration.
 */
@Aspect
@Component
public class AuditAspect {

    private final AuditLogRepository auditLogRepository;

    public AuditAspect(AuditLogRepository auditLogRepository) {
        this.auditLogRepository = auditLogRepository;
    }

    @Around("@annotation(audited)")
    public Object audit(ProceedingJoinPoint joinPoint, Audited audited) throws Throwable {
        String username = currentUsername();
        String action = audited.value();

        try {
            Object result = joinPoint.proceed();
            save(username, action, describeResult(result, joinPoint.getArgs()), true, null);
            return result;
        } catch (Throwable ex) {
            save(username, action, describeArgs(joinPoint.getArgs()), false, ex.getMessage());
            throw ex; // never swallow -- the caller must still see the real failure
        }
    }

    private String currentUsername() {
        Authentication auth = SecurityContextHolder.getContext().getAuthentication();
        return (auth != null && auth.getName() != null) ? auth.getName() : "unknown";
    }

    /** Prefer describing the method's return value (e.g. the saved/updated entity); fall back to args. */
    private String describeResult(Object result, Object[] args) {
        return result != null ? summarize(result) : describeArgs(args);
    }

    private String describeArgs(Object[] args) {
        return Arrays.stream(args).map(this::summarize).collect(Collectors.joining(", "));
    }

    private String summarize(Object o) {
        if (o == null) {
            return "null";
        }
        if (o instanceof Locality l) {
            return "Locality(id=" + l.getId() + ", name=" + l.getName() + ", status=" + l.getStatus() + ")";
        }
        return o.toString();
    }

    private void save(String username, String action, String detail, boolean success, String errorMessage) {
        auditLogRepository.save(new AuditLog(username, action, detail, success, errorMessage, Instant.now()));
    }
}
