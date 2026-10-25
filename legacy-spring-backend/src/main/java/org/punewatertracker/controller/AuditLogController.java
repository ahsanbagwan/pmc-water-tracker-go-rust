package org.punewatertracker.controller;

import org.punewatertracker.audit.AuditLog;
import org.punewatertracker.audit.AuditLogRepository;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.RequestMapping;
import org.springframework.web.bind.annotation.RestController;

import java.util.List;

/** Admin-only: endpoint access is restricted to ROLE_ADMIN in SecurityConfig. */
@RestController
@RequestMapping("/api/audit-log")
public class AuditLogController {

    private final AuditLogRepository auditLogRepository;

    public AuditLogController(AuditLogRepository auditLogRepository) {
        this.auditLogRepository = auditLogRepository;
    }

    @GetMapping
    public List<AuditLog> list() {
        return auditLogRepository.findAllByOrderByTimestampDesc();
    }
}
