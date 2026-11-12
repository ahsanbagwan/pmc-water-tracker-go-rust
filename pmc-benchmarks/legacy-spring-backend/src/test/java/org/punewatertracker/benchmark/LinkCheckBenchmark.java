package org.punewatertracker.benchmark;

import org.junit.jupiter.api.Assumptions;
import org.junit.jupiter.api.Test;
import org.punewatertracker.model.Locality;
import org.punewatertracker.repository.LocalityRepository;
import org.punewatertracker.service.SourceLinkHealthChecker;

import java.util.ArrayList;
import java.util.List;
import java.util.concurrent.CountDownLatch;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import java.util.concurrent.TimeUnit;

import static org.mockito.ArgumentMatchers.any;
import static org.mockito.Mockito.mock;
import static org.mockito.Mockito.withSettings;
import static org.mockito.Mockito.when;

/**
 * Spring twin of the Go and Rust link-check benchmarks: the REAL {@link SourceLinkHealthChecker}
 * checks 1,000 links against a LOCAL stub server (go-api/cmd/linkstub), no internet involved.
 *
 * It is a plain JUnit test, not JMH, so it needs no extra dependency. It skips itself unless
 * STUB_URL is set, so a normal "mvn test" is unaffected. Run it with bench/run.sh, or:
 *
 *   STUB_URL=http://127.0.0.1:9099 mvn test -Dtest=LinkCheckBenchmark
 *
 * Same workload as Go and Rust: every 10th link answers 404, the rest 200; delay 0s and 5ms;
 * concurrency 10 / 50 / 100 (here: the size of the executor's thread pool). Every run verifies
 * each saved result, so a dead stub fails loudly instead of producing a fast, meaningless number.
 * JIT warm-up is run first and discarded, because the JVM is slow until it has compiled the hot path.
 */
class LinkCheckBenchmark {

    private static final int LINKS = 1000;
    private static final int WARMUP = 3;
    private static final int MEASURED = 5;

    /** Mutable holder so ONE checker (one HttpClient, like Go's and Rust's reused clients) serves every iteration. */
    private static final class Run {
        volatile List<Locality> localities = List.of();
        volatile CountDownLatch saved = new CountDownLatch(0);
    }

    @Test
    void linkCheckThroughput() throws Exception {
        String base = System.getenv("STUB_URL");
        Assumptions.assumeTrue(base != null && !base.isBlank(),
                "STUB_URL not set - start the stub (go run ./cmd/linkstub) and set STUB_URL to run this benchmark");

        StringBuilder report = new StringBuilder();
        report.append(String.format("%-28s %12s %12s %12s%n", "config", "mean ms/op", "best ms/op", "links/s"));

        for (String delay : new String[]{"0s", "5ms"}) {
            for (int concurrency : new int[]{10, 50, 100}) {
                // The production executor is core 4 / max 8 / queue 100 (TaskExecutorConfig) - it would reject
                // a 1,000-task burst, so the benchmark uses a fixed pool of `concurrency` threads, unbounded queue.
                ExecutorService executor = Executors.newFixedThreadPool(concurrency);
                Run run = new Run();
                LocalityRepository repository = mock(LocalityRepository.class, withSettings().stubOnly());
                when(repository.findAll()).thenAnswer(inv -> run.localities);
                when(repository.save(any(Locality.class))).thenAnswer(inv -> {
                    run.saved.countDown();
                    return inv.getArgument(0);
                });
                SourceLinkHealthChecker checker = new SourceLinkHealthChecker(repository, executor);

                double totalMs = 0;
                double bestMs = Double.MAX_VALUE;
                for (int i = 0; i < WARMUP + MEASURED; i++) {
                    run.localities = buildLocalities(base, delay);
                    run.saved = new CountDownLatch(LINKS);

                    long start = System.nanoTime();
                    checker.triggerManualCheck();
                    if (!run.saved.await(120, TimeUnit.SECONDS)) {
                        throw new AssertionError("timed out waiting for " + LINKS + " results (delay=" + delay
                                + ", concurrency=" + concurrency + ", still missing " + run.saved.getCount() + ")");
                    }
                    double ms = (System.nanoTime() - start) / 1_000_000.0;

                    verify(run.localities);
                    if (i >= WARMUP) {
                        totalMs += ms;
                        bestMs = Math.min(bestMs, ms);
                    }
                }
                executor.shutdownNow();

                double meanMs = totalMs / MEASURED;
                report.append(String.format("%-28s %12.1f %12.1f %12.0f%n",
                        "delay=" + delay + "/conc=" + concurrency, meanMs, bestMs, LINKS / (meanMs / 1000.0)));
            }
        }
        System.out.println("\n=== Spring: SourceLinkHealthChecker, " + LINKS + " links ===\n" + report);
    }

    private static List<Locality> buildLocalities(String base, String delay) {
        List<Locality> list = new ArrayList<>(LINKS);
        for (int i = 0; i < LINKS; i++) {
            Locality l = new Locality();
            l.setId((long) i);
            l.setName("Locality " + i);
            int code = (i % 10 == 0) ? 404 : 200;
            l.setSourceUrl(base + "/status/" + code + "?delay=" + delay);
            list.add(l);
        }
        return list;
    }

    private static void verify(List<Locality> localities) {
        for (Locality l : localities) {
            boolean expected = l.getId() % 10 != 0;
            if (!Boolean.valueOf(expected).equals(l.getSourceUrlHealthy())) {
                throw new AssertionError("locality " + l.getId() + ": healthy=" + l.getSourceUrlHealthy()
                        + ", expected " + expected + " - is the stub reachable at STUB_URL?");
            }
        }
    }
}
