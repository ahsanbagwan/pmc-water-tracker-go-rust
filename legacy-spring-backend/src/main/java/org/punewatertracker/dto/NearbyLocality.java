package org.punewatertracker.dto;

import org.punewatertracker.model.Locality;

public record NearbyLocality(Locality locality, double distanceKm) {}
