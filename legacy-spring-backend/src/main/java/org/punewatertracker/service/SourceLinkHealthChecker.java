package org.punewatertracker.service;

import org.punewatertracker.audit.Audited;
import org.punewatertracker.model.Locality;
import org.punewatertracker.repository.LocalityRepository;
import net.javacrumbs.shedlock.spring.annotation.SchedulerLock;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.beans.factory.annotation.Qualifier;
import org.springframework.scheduling.annotation.Scheduled;
import org.springframework.stereotype.Service;

import java.net.URI;
import java.net.http.HttpClient;
import java.net.http.HttpRequest;
import java.net.http.HttpResponse;
import java.time.Duration;
import java.time.Instant;
import java.util.List;
import java.util.concurrent.CompletableFuture;
import java.util.concurrent.Executor;

@Service
public class SourceLinkHealthChecker {

    private static final Logger log = LoggerFactory.getLogger(SourceLinkHealthChecker.class);

    private final LocalityRepository repository;
    private final Executor executor;
    private final HttpClient httpClient = HttpClient.newBuilder()
            .connectTimeout(Duration.ofSeconds(8))
            .followRedirects(HttpClient.Redirect.NORMAL)
            .build();

    public SourceLinkHealthChecker(LocalityRepository repository,
                                    @Qualifier("sourceLinkCheckExecutor") Executor executor) {
        this.repository = repository;
        this.executor = executor;
    }

    /** Weekly, Monday 03:00 server time -- sources don't rot fast, and this is off-hours.
     *  @SchedulerLock ensures only ONE pod actually runs this per scheduled fire, even though
     *  every pod's own Spring context independently reaches this @Scheduled trigger at the
     *  same time -- without it, N replicas would mean N redundant runs, each hitting every
     *  locality's source URL independently. lockAtLeastFor prevents a suspiciously-fast pod
     *  clock skew from letting a second pod grab the lock again moments after the first
     *  finished; lockAtMostFor (30m, from SchedulerLockConfig's default) is the safety-net
     *  ceiling if a pod died mid-run without releasing the lock. */
    @Scheduled(cron = "${app.source-link-check-cron:0 0 3 * * MON}")
    @SchedulerLock(name = "sourceLinkHealthCheck", lockAtLeastFor = "5m")
    public void scheduledCheck() {
        checkAllAsync();
    }

    /** Fire-and-forget: kicks off the check on the dedicated pool and returns immediately. */
    @Audited("CHECK_SOURCE_LINKS")
    public void triggerManualCheck() {
        checkAllAsync();
    }

    private void checkAllAsync() {
        List<Locality> withSources = repository.findAll().stream()
                .filter(l -> l.getSourceUrl() != null && !l.getSourceUrl().isBlank())
                .toList();

        List<CompletableFuture<Void>> checks = withSources.stream()
                .map(locality -> CompletableFuture.runAsync(() -> checkOne(locality), executor))
                .toList();

        CompletableFuture.allOf(checks.toArray(new CompletableFuture[0]))
                .thenRun(() -> logSummary(withSources))
                .exceptionally(ex -> {
                    log.error("Source-link check batch failed unexpectedly", ex);
                    return null;
                });
    }

    private void checkOne(Locality locality) {
        boolean healthy;
        try {
            HttpRequest request = HttpRequest.newBuilder()
                    .uri(URI.create(locality.getSourceUrl()))
                    .timeout(Duration.ofSeconds(8))
                    .GET()
                    .build();
            HttpResponse<Void> response = httpClient.send(request, HttpResponse.BodyHandlers.discarding());
            healthy = response.statusCode() < 400;
        } catch (Exception ex) {
            healthy = false;
        }

        locality.setSourceUrlHealthy(healthy);
        locality.setSourceUrlLastChecked(Instant.now());
        repository.save(locality);
    }

    private void logSummary(List<Locality> checked) {
        long broken = checked.stream().filter(l -> Boolean.FALSE.equals(l.getSourceUrlHealthy())).count();
        log.info("Checked {} source links, {} appear broken.", checked.size(), broken);
        checked.stream()
                .filter(l -> Boolean.FALSE.equals(l.getSourceUrlHealthy()))
                .forEach(l -> log.warn("Broken source link: {} -> {}", l.getName(), l.getSourceUrl()));
    }
}
