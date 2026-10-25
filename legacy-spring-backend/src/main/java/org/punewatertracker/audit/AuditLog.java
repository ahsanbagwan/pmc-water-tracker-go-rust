package org.punewatertracker.audit;

import jakarta.persistence.*;

import java.time.Instant;

@Entity
@Table(name = "audit_log")
public class AuditLog {

    @Id
    @GeneratedValue(strategy = GenerationType.IDENTITY)
    private Long id;

    @Column(nullable = false)
    private String username;

    @Column(nullable = false)
    private String action;

    @Column(length = 1000)
    private String detail;

    @Column(nullable = false)
    private boolean success;

    @Column(length = 1000)
    private String errorMessage;

    @Column(nullable = false)
    private Instant timestamp;

    public AuditLog() {
    }

    public AuditLog(String username, String action, String detail, boolean success, String errorMessage, Instant timestamp) {
        this.username = username;
        this.action = action;
        this.detail = detail;
        this.success = success;
        this.errorMessage = errorMessage;
        this.timestamp = timestamp;
    }

    public Long getId() {
        return id;
    }

    public String getUsername() {
        return username;
    }

    public String getAction() {
        return action;
    }

    public String getDetail() {
        return detail;
    }

    public boolean isSuccess() {
        return success;
    }

    public String getErrorMessage() {
        return errorMessage;
    }

    public Instant getTimestamp() {
        return timestamp;
    }
}
