package org.punewatertracker.model;

/** How urgent a citizen's report sounds, based on their description. Lets admins triage
 *  the pending-reports queue by severity instead of reading everything serially. */
public enum ReportUrgency {
    UNKNOWN,
    LOW,
    MEDIUM,
    HIGH,
    CRITICAL
}
