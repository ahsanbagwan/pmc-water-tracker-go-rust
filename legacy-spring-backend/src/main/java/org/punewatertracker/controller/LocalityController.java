package org.punewatertracker.controller;

import jakarta.validation.Valid;
import org.punewatertracker.dto.NearbyLocality;
import org.punewatertracker.model.Locality;
import org.punewatertracker.model.WaterStatus;
import org.punewatertracker.service.LocalityService;
import org.punewatertracker.service.SourceLinkHealthChecker;
import org.springframework.http.HttpStatus;
import org.springframework.http.ResponseEntity;
import org.springframework.web.bind.annotation.*;

import java.util.List;

@RestController
@RequestMapping("/api/localities")
public class LocalityController {

    private final LocalityService service;
    private final SourceLinkHealthChecker sourceLinkHealthChecker;

    public LocalityController(LocalityService service, SourceLinkHealthChecker sourceLinkHealthChecker) {
        this.service = service;
        this.sourceLinkHealthChecker = sourceLinkHealthChecker;
    }

    /** GET /api/localities?status=TANKER_DEPENDENT&search=wagholi */
    @GetMapping
    public List<Locality> list(
            @RequestParam(required = false) WaterStatus status,
            @RequestParam(required = false) String search) {
        return service.findVisible(status, search);
    }

    @GetMapping("/{id}")
    public Locality get(@PathVariable Long id) {
        return service.findById(id);
    }

    /** GET /api/localities/nearby?lat=18.55&lng=73.85&radiusKm=5 (radiusKm optional -- omit for no cutoff) */
    @GetMapping("/nearby")
    public List<NearbyLocality> nearby(
            @RequestParam double lat,
            @RequestParam double lng,
            @RequestParam(required = false) Double radiusKm) {
        return service.findNearby(lat, lng, radiusKm);
    }

    /** How far to the closest reliable (MUNICIPAL) supply from this locality. */
    @GetMapping("/{id}/nearest-reliable")
    public ResponseEntity<NearbyLocality> nearestReliable(@PathVariable Long id) {
        NearbyLocality result = service.findNearestReliableSupply(id);
        return result != null ? ResponseEntity.ok(result) : ResponseEntity.noContent().build();
    }

    /** Admin-curated addition, treated as verified immediately. */
    @PostMapping
    public ResponseEntity<Locality> create(@Valid @RequestBody Locality locality) {
        return ResponseEntity.status(HttpStatus.CREATED).body(service.createVerified(locality));
    }

    /** Citizen correction/report — queued for admin review, not shown publicly until approved. */
    @PostMapping("/reports")
    public ResponseEntity<Locality> report(@Valid @RequestBody Locality locality) {
        return ResponseEntity.status(HttpStatus.CREATED).body(service.createUnverifiedReport(locality));
    }

    @GetMapping("/reports/pending")
    public List<Locality> pendingReports() {
        return service.findPendingReports();
    }

    @PostMapping("/reports/{id}/approve")
    public Locality approve(@PathVariable Long id) {
        return service.approveReport(id);
    }

    @PutMapping("/{id}")
    public Locality update(@PathVariable Long id, @Valid @RequestBody Locality locality) {
        return service.update(id, locality);
    }

    @DeleteMapping("/{id}")
    public ResponseEntity<Void> delete(@PathVariable Long id) {
        service.delete(id);
        return ResponseEntity.noContent().build();
    }

    /**
     * Kicks off a concurrent check of every locality's source URL and returns immediately --
     * the check itself runs on a dedicated thread pool, not the request thread. Also runs
     * automatically every Monday (see SourceLinkHealthChecker).
     */
    @PostMapping("/check-source-links")
    public ResponseEntity<Void> checkSourceLinks() {
        sourceLinkHealthChecker.triggerManualCheck();
        return ResponseEntity.accepted().build();
    }
}
