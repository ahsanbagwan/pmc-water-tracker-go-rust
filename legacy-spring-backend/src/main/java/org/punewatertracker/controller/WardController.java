package org.punewatertracker.controller;

import org.punewatertracker.audit.Audited;
import org.punewatertracker.model.Locality;
import org.punewatertracker.service.LocalityService;
import org.punewatertracker.service.WardBoundaryService;
import org.springframework.web.bind.annotation.*;

import java.util.Map;

@RestController
@RequestMapping("/api/wards")
public class WardController {

    private final WardBoundaryService wardBoundaryService;
    private final LocalityService localityService;

    public WardController(WardBoundaryService wardBoundaryService, LocalityService localityService) {
        this.wardBoundaryService = wardBoundaryService;
        this.localityService = localityService;
    }

    /** GET /api/wards/lookup?lat=18.55&lng=73.85 -- doesn't touch any stored data, just answers
     *  "which ward is this point in." Public: same sensitivity level as any other GET here. */
    @GetMapping("/lookup")
    public Map<String, Object> lookup(@RequestParam double lat, @RequestParam double lng) {
        String ward = wardBoundaryService.findWard(lat, lng);
        return Map.of(
                "ward", ward == null ? "" : ward,
                "found", ward != null,
                "loadedWardCount", wardBoundaryService.loadedWardCount()
        );
    }

    /** Recomputes and persists one locality's ward from its actual coordinates -- deliberately
     *  a manual, per-locality, admin-triggered action rather than a silent bulk overwrite, so
     *  an admin can review before an existing (possibly hand-verified) ward value gets replaced. */
    @Audited("RECOMPUTE_WARD")
    @PutMapping("/{localityId}/recompute")
    public Locality recompute(@PathVariable Long localityId) {
        Locality locality = localityService.findById(localityId);
        if (locality.getLatitude() != null && locality.getLongitude() != null) {
            String ward = wardBoundaryService.findWard(locality.getLatitude(), locality.getLongitude());
            if (ward != null) {
                locality.setWard(ward);
            }
        }
        return localityService.update(localityId, locality);
    }
}
