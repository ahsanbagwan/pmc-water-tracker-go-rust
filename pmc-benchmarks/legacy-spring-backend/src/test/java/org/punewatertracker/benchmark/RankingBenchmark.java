package org.punewatertracker.benchmark;

import org.junit.jupiter.api.Assumptions;
import org.junit.jupiter.api.Test;
import org.punewatertracker.dto.NearbyLocality;
import org.punewatertracker.model.Locality;
import org.punewatertracker.service.GeoUtils;

import java.util.ArrayList;
import java.util.Comparator;
import java.util.List;

/**
 * Spring twin of Go's BenchmarkRankLocal and Rust's rank_nearby benchmark: nearby ranking over the
 * same 1,000 points. The pipeline below is a copy of LocalityService.findNearby (filter on coordinates,
 * map with GeoUtils.distanceKm, filter by radius, sort), minus the repository call - that method needs
 * a database-backed repository, which would measure the database instead of the ranking.
 *
 * Plain JUnit, no JMH dependency. Runs only when BENCH=1 (bench/run.sh sets it).
 */
class RankingBenchmark {

    private static final double LAT = 18.5204;
    private static final double LNG = 73.8567;

    @Test
    void nearbyRanking() {
        Assumptions.assumeTrue("1".equals(System.getenv("BENCH")), "set BENCH=1 to run this benchmark");

        List<Locality> points = new ArrayList<>(1000);
        for (int i = 0; i < 1000; i++) {
            Locality l = new Locality();
            l.setId((long) i);
            l.setName("P" + i);
            l.setLatitude(18.40 + (i * 7919 % 1000) / 4000.0);   // same formula as the Go and Rust benchmarks
            l.setLongitude(73.70 + (i * 104729 % 1000) / 4000.0);
            points.add(l);
        }

        StringBuilder out = new StringBuilder(String.format("%-24s %12s%n", "case", "ns/op"));
        out.append(String.format("%-24s %12.0f%n", "all", timeNsPerOp(() -> rank(points, null, 0))));
        out.append(String.format("%-24s %12.0f%n", "radius5km_limit10", timeNsPerOp(() -> rank(points, 5.0, 10))));
        System.out.println("\n=== Spring: nearby ranking over 1,000 points ===\n" + out);
    }

    private static List<NearbyLocality> rank(List<Locality> points, Double radiusKm, int limit) {
        List<NearbyLocality> ranked = points.stream()
                .filter(loc -> loc.getLatitude() != null && loc.getLongitude() != null)
                .map(loc -> new NearbyLocality(loc, GeoUtils.distanceKm(LAT, LNG, loc.getLatitude(), loc.getLongitude())))
                .filter(n -> radiusKm == null || n.distanceKm() <= radiusKm)
                .sorted(Comparator.comparingDouble(NearbyLocality::distanceKm))
                .toList();
        return limit > 0 && ranked.size() > limit ? ranked.subList(0, limit) : ranked;
    }

    private static volatile Object sink; // keeps the JIT from discarding the work

    /** Warm up for ~2s so the JIT has compiled the hot path, then average ~2s of runs. */
    private static double timeNsPerOp(java.util.function.Supplier<Object> op) {
        long warmupEnd = System.nanoTime() + 2_000_000_000L;
        while (System.nanoTime() < warmupEnd) {
            sink = op.get();
        }
        long start = System.nanoTime();
        long end = start + 2_000_000_000L;
        long iterations = 0;
        while (System.nanoTime() < end) {
            sink = op.get();
            iterations++;
        }
        return (System.nanoTime() - start) / (double) iterations;
    }
}
