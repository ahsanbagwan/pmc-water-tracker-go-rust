package org.punewatertracker.audit;

import java.lang.annotation.ElementType;
import java.lang.annotation.Retention;
import java.lang.annotation.RetentionPolicy;
import java.lang.annotation.Target;

/**
 * Marks a method as security/data-sensitive enough to record who called it, when, with what,
 * and whether it succeeded. Picked up by AuditAspect -- the method itself needs no logging code.
 */
@Retention(RetentionPolicy.RUNTIME)
@Target(ElementType.METHOD)
public @interface Audited {
    /** Short action name, e.g. "DELETE_LOCALITY" -- shows up as-is in the audit log. */
    String value();
}
