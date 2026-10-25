package org.punewatertracker.service;

import org.punewatertracker.audit.Audited;
import org.punewatertracker.config.CacheConfig;
import org.punewatertracker.dto.NearbyLocality;
import org.punewatertracker.messaging.ReportEventPublisher;
import org.punewatertracker.messaging.ReportSubmittedEvent;
import org.punewatertracker.model.Locality;
import org.punewatertracker.model.WaterStatus;
import org.punewatertracker.repository.LocalityRepository;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.cache.annotation.CacheEvict;
import org.springframework.cache.annotation.Cacheable;
import org.springframework.stereotype.Service;

import java.time.Instant;
import java.time.LocalDate;
import java.util.Comparator;
import java.util.List;
import java.util.NoSuchElementException;

@Service
public class LocalityService {

    private static final Logger log = LoggerFactory.getLogger(LocalityService.class);

    private final LocalityRepository repository;
    private final ReportEventPublisher reportEventPublisher;
    private final NearbyReportFinder nearbyReportFinder;
    private final OllamaReportAnalyzer ollamaReportAnalyzer;

    public LocalityService(LocalityRepository repository, ReportEventPublisher reportEventPublisher,
                            NearbyReportFinder nearbyReportFinder, OllamaReportAnalyzer ollamaReportAnalyzer) {
        this.repository = repository;
        this.reportEventPublisher = reportEventPublisher;
        this.nearbyReportFinder = nearbyReportFinder;
        this.ollamaReportAnalyzer = ollamaReportAnalyzer;
    }

    /** Publicly visible localities: verified entries only, optionally narrowed by status or name. */
    @Cacheable(value = CacheConfig.LOCALITIES_CACHE, key = "'list:status=' + #status + ',search=' + #search")
    public List<Locality> findVisible(WaterStatus status, String search) {
        if (status != null) {
            return repository.findByStatus(status).stream()
                    .filter(Locality::isVerified)
                    .toList();
        }
        if (search != null && !search.isBlank()) {
            return repository.findByNameContainingIgnoreCaseAndVerifiedTrue(search);
        }
        return repository.findByVerifiedTrue();
    }

    /**
     * Verified localities within radiusKm of (lat, lng), sorted nearest-first. radiusKm null
     * means no cutoff -- just sorted by distance, letting the caller (e.g. a "top 10 nearest")
     * decide how to trim the list rather than baking a default radius in here.
     */
    public List<NearbyLocality> findNearby(double lat, double lng, Double radiusKm) {
        return repository.findByVerifiedTrue().stream()
                .filter(loc -> loc.getLatitude() != null && loc.getLongitude() != null)
                .map(loc -> new NearbyLocality(loc, GeoUtils.distanceKm(lat, lng, loc.getLatitude(), loc.getLongitude())))
                .filter(nearby -> radiusKm == null || nearby.distanceKm() <= radiusKm)
                .sorted(Comparator.comparingDouble(NearbyLocality::distanceKm))
                .toList();
    }

    /** The closest MUNICIPAL-status locality to the given one -- makes "how far to reliable
     *  water" concrete rather than abstract when viewing a tanker-dependent area. Excludes the
     *  locality itself. Null if it has no coordinates, or no other MUNICIPAL locality exists. */
    public NearbyLocality findNearestReliableSupply(Long localityId) {
        Locality origin = findById(localityId);
        if (origin.getLatitude() == null || origin.getLongitude() == null) {
            return null;
        }

        return repository.findByStatus(WaterStatus.MUNICIPAL).stream()
                .filter(Locality::isVerified)
                .filter(loc -> !loc.getId().equals(localityId))
                .filter(loc -> loc.getLatitude() != null && loc.getLongitude() != null)
                .map(loc -> new NearbyLocality(loc, GeoUtils.distanceKm(
                        origin.getLatitude(), origin.getLongitude(), loc.getLatitude(), loc.getLongitude())))
                .min(Comparator.comparingDouble(NearbyLocality::distanceKm))
                .orElse(null);
    }

    @Cacheable(value = CacheConfig.LOCALITIES_CACHE, key = "'id:' + #id")
    public Locality findById(Long id) {
        return repository.findById(id)
                .orElseThrow(() -> new NoSuchElementException("No locality with id " + id));
    }

    /** Admin-curated entry, backed by a checkable source — goes live immediately. */
    @Audited("CREATE_LOCALITY")
    @CacheEvict(value = CacheConfig.LOCALITIES_CACHE, allEntries = true)
    public Locality createVerified(Locality locality) {
        locality.setId(null);
        locality.setVerified(true);
        if (locality.getLastVerified() == null) {
            locality.setLastVerified(LocalDate.now());
        }
        Locality saved = repository.save(locality);
        log.info("Created verified locality '{}' (id={}, status={})", saved.getName(), saved.getId(), saved.getStatus());
        return saved;
    }

    /** Citizen-submitted correction/report — held back from the public view until an admin confirms it.
     *  Deliberately doesn't evict the cache: unverified entries never appear in findVisible's
     *  results, so nothing cached actually changes here. */
    public Locality createUnverifiedReport(Locality locality) {
        locality.setId(null);
        locality.setVerified(false);
        locality.setLastVerified(LocalDate.now());

        if (locality.getCitizenDescription() != null && !locality.getCitizenDescription().isBlank()) {
            // Cheap deterministic prefilter first -- only spend an Ollama call asking about a
            // duplicate when there's actually a geo/time-nearby candidate to compare against.
            Locality nearbyCandidate = nearbyReportFinder.findClosestCandidate(locality);
            String candidateDescription = nearbyCandidate != null ? nearbyCandidate.getCitizenDescription() : null;

            OllamaReportAnalyzer.Analysis analysis =
                    ollamaReportAnalyzer.analyze(locality.getCitizenDescription(), candidateDescription);

            if (analysis != null) {
                if (locality.getReportedUrgency() == null) {
                    locality.setReportedUrgency(analysis.urgency());
                    locality.setUrgencyScore(analysis.urgencyConfidence());
                    locality.setUrgencySignals(analysis.urgencyReasoning());
                }
                if (nearbyCandidate != null && analysis.isDuplicate()) {
                    locality.setPossibleDuplicateOfId(nearbyCandidate.getId());
                    locality.setDuplicateConfidence(analysis.duplicateConfidence());
                }
            }
        }

        Locality saved = repository.save(locality);

        log.info("Citizen report submitted for '{}' (id={}, proposedStatus={}){}",
                saved.getName(), saved.getId(), saved.getStatus(),
                saved.getReportedUrgency() != null
                        ? ", urgency=" + saved.getReportedUrgency() + " (score=" + saved.getUrgencyScore() + ")"
                        : "");
        if (saved.getPossibleDuplicateOfId() != null) {
            log.info("Report id={} flagged as possible duplicate of id={} (confidence={})",
                    saved.getId(), saved.getPossibleDuplicateOfId(), saved.getDuplicateConfidence());
        }

        reportEventPublisher.publishReportSubmitted(new ReportSubmittedEvent(
                saved.getId(), saved.getName(), saved.getStatus().name(), saved.getNotes(), Instant.now()));

        return saved;
    }

    /** Sorted by urgency confidence score (most severe first) so admins triage instead of reading serially. */
    public List<Locality> findPendingReports() {
        return repository.findAll().stream()
                .filter(l -> !l.isVerified())
                .sorted((a, b) -> scoreOrZero(b) - scoreOrZero(a))
                .toList();
    }

    private int scoreOrZero(Locality locality) {
        return locality.getUrgencyScore() != null ? locality.getUrgencyScore() : 0;
    }

    @Audited("APPROVE_REPORT")
    @CacheEvict(value = CacheConfig.LOCALITIES_CACHE, allEntries = true)
    public Locality approveReport(Long id) {
        Locality locality = findById(id);
        locality.setVerified(true);
        Locality saved = repository.save(locality);
        log.info("Approved report for '{}' (id={}) -- now publicly visible", saved.getName(), saved.getId());
        return saved;
    }

    @Audited("UPDATE_LOCALITY")
    @CacheEvict(value = CacheConfig.LOCALITIES_CACHE, allEntries = true)
    public Locality update(Long id, Locality patch) {
        Locality existing = findById(id);
        existing.setName(patch.getName());
        existing.setWard(patch.getWard());
        existing.setStatus(patch.getStatus());
        existing.setLatitude(patch.getLatitude());
        existing.setLongitude(patch.getLongitude());
        existing.setNotes(patch.getNotes());
        existing.setSourceName(patch.getSourceName());
        existing.setSourceUrl(patch.getSourceUrl());
        existing.setLastVerified(patch.getLastVerified() != null ? patch.getLastVerified() : LocalDate.now());
        return repository.save(existing);
    }

    @Audited("DELETE_LOCALITY")
    @CacheEvict(value = CacheConfig.LOCALITIES_CACHE, allEntries = true)
    public void delete(Long id) {
        log.warn("Deleting locality id={}", id);
        repository.deleteById(id);
    }
}
