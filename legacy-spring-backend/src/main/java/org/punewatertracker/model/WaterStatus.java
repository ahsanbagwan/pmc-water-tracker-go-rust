package org.punewatertracker.model;

/**
 * The water-supply state of a locality.
 *
 * MUNICIPAL            - Reliable piped PMC connection, no regular tanker dependency.
 * TANKER_DEPENDENT     - No functioning piped connection; residents rely on tankers/borewells.
 * MIXED                - Partial piped coverage; some streets/societies still on tankers.
 * PIPELINE_IN_PROGRESS - Pipeline-laying or connection work is actively underway (e.g. under the
 *                        24x7 water supply project) but not yet delivering water to residents.
 */
public enum WaterStatus {
    MUNICIPAL,
    TANKER_DEPENDENT,
    MIXED,
    PIPELINE_IN_PROGRESS
}
