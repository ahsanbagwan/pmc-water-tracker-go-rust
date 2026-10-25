package org.punewatertracker.service;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import jakarta.annotation.PostConstruct;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.core.io.ClassPathResource;
import org.springframework.stereotype.Service;

import java.io.InputStream;
import java.util.ArrayList;
import java.util.List;

/**
 * Point-in-polygon lookup against PMC's 15 administrative ward boundaries (the ones tied to
 * actual ward offices citizens go to -- not the 41 electoral wards, which redraw every
 * election and aren't published as boundary data anywhere findable).
 *
 * Source: datameet/Pune_wards (MIT / CC-BY-SA-2.5-India, hand-traced from official PMC ward
 * PDFs) -- https://github.com/datameet/Pune_wards/blob/master/GeoData/pune-admin-wards.geojson
 * Download that file and place it at backend/src/main/resources/static/pune-admin-wards.geojson
 * (also makes it servable to the frontend at GET /pune-admin-wards.geojson for drawing on the
 * map, for free, via Spring Boot's static resource handling -- one file, two uses).
 *
 * If the file isn't present, this degrades to "no wards loaded" -- findWard() always returns
 * null -- rather than failing application startup. Ward lookup is an enrichment, not something
 * that should ever block the app from running.
 */
@Service
public class WardBoundaryService {

    private static final Logger log = LoggerFactory.getLogger(WardBoundaryService.class);

    private record Ring(double[] lats, double[] lngs) {}
    private record Ward(String name, List<Ring> rings) {}

    private final List<Ward> wards = new ArrayList<>();

    @PostConstruct
    public void loadBoundaries() {
        ClassPathResource resource = new ClassPathResource("static/pune-admin-wards.geojson");
        if (!resource.exists()) {
            log.info("pune-admin-wards.geojson not found under src/main/resources/static/ -- "
                    + "ward lookup disabled until it's added. See WardBoundaryService's class "
                    + "comment for where to get it.");
            return;
        }

        try (InputStream in = resource.getInputStream()) {
            ObjectMapper mapper = new ObjectMapper();
            JsonNode root = mapper.readTree(in);
            JsonNode features = root.path("features");

            for (JsonNode feature : features) {
                String name = extractName(feature.path("properties"));
                List<Ring> rings = extractRings(feature.path("geometry"));
                if (name != null && !rings.isEmpty()) {
                    wards.add(new Ward(name, rings));
                }
            }
            log.info("Loaded {} ward boundaries.", wards.size());
        } catch (Exception ex) {
            log.error("Failed to parse pune-admin-wards.geojson -- ward lookup disabled.", ex);
        }
    }

    /** Which admin ward (lat, lng) falls inside, or null if outside all loaded boundaries
     *  (including the case where no boundaries are loaded at all). */
    public String findWard(double lat, double lng) {
        for (Ward ward : wards) {
            for (Ring ring : ward.rings()) {
                if (containsPoint(ring, lat, lng)) {
                    return ward.name();
                }
            }
        }
        return null;
    }

    public int loadedWardCount() {
        return wards.size();
    }

    private String extractName(JsonNode properties) {
        // Different PMC ward datasets use different property keys for the name -- check the
        // common ones rather than assuming one exact key.
        for (String key : List.of("name", "Name", "NAME", "ward_name", "WARD_NAME")) {
            if (properties.has(key) && !properties.path(key).asText().isBlank()) {
                return properties.path(key).asText();
            }
        }
        return null;
    }

    /** Handles both Polygon and MultiPolygon geometries; ignores holes (interior rings) --
     *  fine for this use case since ward boundaries in this dataset are simple exterior shapes. */
    private List<Ring> extractRings(JsonNode geometry) {
        List<Ring> rings = new ArrayList<>();
        String type = geometry.path("type").asText();
        JsonNode coordinates = geometry.path("coordinates");

        if ("Polygon".equals(type)) {
            addRingFromExteriorArray(rings, coordinates.get(0));
        } else if ("MultiPolygon".equals(type)) {
            for (JsonNode polygon : coordinates) {
                addRingFromExteriorArray(rings, polygon.get(0));
            }
        }
        return rings;
    }

    private void addRingFromExteriorArray(List<Ring> rings, JsonNode exteriorRing) {
        if (exteriorRing == null) {
            return;
        }
        int size = exteriorRing.size();
        double[] lats = new double[size];
        double[] lngs = new double[size];
        for (int i = 0; i < size; i++) {
            // GeoJSON coordinate order is [longitude, latitude] -- easy to get backwards.
            lngs[i] = exteriorRing.get(i).get(0).asDouble();
            lats[i] = exteriorRing.get(i).get(1).asDouble();
        }
        rings.add(new Ring(lats, lngs));
    }

    /** Standard ray-casting point-in-polygon test: count how many times a ray from the point
     *  crosses the polygon's edges: odd = inside, even = outside. */
    private boolean containsPoint(Ring ring, double lat, double lng) {
        boolean inside = false;
        int n = ring.lats().length;
        for (int i = 0, j = n - 1; i < n; j = i++) {
            double latI = ring.lats()[i], lngI = ring.lngs()[i];
            double latJ = ring.lats()[j], lngJ = ring.lngs()[j];

            boolean crosses = ((latI > lat) != (latJ > lat))
                    && (lng < (lngJ - lngI) * (lat - latI) / (latJ - latI) + lngI);
            if (crosses) {
                inside = !inside;
            }
        }
        return inside;
    }
}
