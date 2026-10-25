package org.punewatertracker.messaging;

import java.time.Instant;

public record ReportSubmittedEvent(
        Long localityId,
        String localityName,
        String proposedStatus,
        String notes,
        Instant submittedAt
) {}
