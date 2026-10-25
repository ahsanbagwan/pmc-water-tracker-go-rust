package org.punewatertracker.model;

/**
 * ADMIN  - full access: create/edit/delete localities, approve reports, manage users.
 * EDITOR - can create/edit localities and approve citizen reports, but cannot delete
 *          entries or manage users.
 */
public enum Role {
    ADMIN,
    EDITOR
}
