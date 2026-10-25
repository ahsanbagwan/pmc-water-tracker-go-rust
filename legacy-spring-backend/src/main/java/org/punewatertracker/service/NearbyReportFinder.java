package org.punewatertracker.service;

import org.punewatertracker.model.Locality;
import org.punewatertracker.repository.LocalityRepository;
import org.springframework.stereotype.Service;

import java.time.LocalDate;
import java.time.temporal.ChronoUnit;
import java.util.Comparator;

/**
 * Cheap deterministic prefilter: finds the closest-in-space-and-time earlier unverified report,
 * if any, so OllamaReportAnalyzer only needs to ask about ONE candidate per submission -- keeps
 * latency bounded regardless of queue size.
 */
@Service
public class NearbyReportFinder {

    private static final double EARTH_RADIUS_KM = 6371.0;
    private static final double MAX_DISTANCE_KM = 1.0;
    private static final long MAX_DAYS = 14;

    private final LocalityRepository repository;

    public NearbyReportFinder(LocalityRepository repository) {
        this.repository = repository;
    }

    public Locality findClosestCandidate(Locality candidate) {
        if (candidate.getLatitude() == null || candidate.getLongitude() == null) {
            return null;
        }
        LocalDate today = LocalDate.now();

        return repository.findAll().stream()
                .filter(existing -> !existing.isVerified())
                .filter(existing -> existing.getId() != null) // excludes the not-yet-saved candidate itself
                .filter(existing -> existing.getLatitude() != null && existing.getLongitude() != null)
                .filter(existing -> existing.getCitizenDescription() != null
                        && !existing.getCitizenDescription().isBlank())
                .filter(existing -> existing.getLastVerified() != null
                        && ChronoUnit.DAYS.between(existing.getLastVerified(), today) <= MAX_DAYS)
                .filter(existing -> distanceKm(candidate.getLatitude(), candidate.getLongitude(),
                        existing.getLatitude(), existing.getLongitude()) <= MAX_DISTANCE_KM)
                .min(Comparator.comparingDouble(existing -> distanceKm(
                        candidate.getLatitude(), candidate.getLongitude(),
                        existing.getLatitude(), existing.getLongitude())))
                .orElse(null);
    }

    private double distanceKm(double lat1, double lon1, double lat2, double lon2) {
        double dLat = Math.toRadians(lat2 - lat1);
        double dLon = Math.toRadians(lon2 - lon1);
        double a = Math.sin(dLat / 2) * Math.sin(dLat / 2)
                + Math.cos(Math.toRadians(lat1)) * Math.cos(Math.toRadians(lat2))
                * Math.sin(dLon / 2) * Math.sin(dLon / 2);
        double c = 2 * Math.atan2(Math.sqrt(a), Math.sqrt(1 - a));
        return EARTH_RADIUS_KM * c;
    }
}
