package org.punewatertracker.service;

import com.sun.net.httpserver.HttpServer;
import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.punewatertracker.model.Locality;
import org.punewatertracker.repository.LocalityRepository;

import java.net.InetSocketAddress;
import java.time.Duration;
import java.util.List;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;

import static org.awaitility.Awaitility.await;
import static org.mockito.ArgumentMatchers.argThat;
import static org.mockito.Mockito.mock;
import static org.mockito.Mockito.verify;
import static org.mockito.Mockito.when;

/**
 * SourceLinkHealthChecker.triggerManualCheck() is fire-and-forget: it kicks off HTTP checks on
 * a background executor and returns immediately, well before the checks actually complete. A
 * naive test asserting right after calling it would run before the background work finishes,
 * making the test pass or fail unpredictably depending on machine speed/load -- exactly the
 * kind of flakiness Awaitility exists to eliminate.
 *
 * await().untilAsserted(...) re-runs the assertion repeatedly until it either passes or the
 * timeout elapses, which is the correct way to test "this eventually becomes true"
 * asynchronous behavior -- cleaner and less brittle than a hand-rolled Thread.sleep() guess.
 *
 * Uses a real local HTTP server (JDK's built-in com.sun.net.httpserver, no new dependency)
 * rather than mocking HttpClient, since SourceLinkHealthChecker constructs its HttpClient
 * internally rather than accepting one as a dependency -- this keeps the test fast and
 * deterministic without needing real internet access or a production refactor.
 */
class SourceLinkHealthCheckerTest {

    private HttpServer testServer;
    private String reachableUrl;
    private ExecutorService checkExecutor;

    @BeforeEach
    void setUp() throws Exception {
        testServer = HttpServer.create(new InetSocketAddress("localhost", 0), 0);
        testServer.createContext("/ok", exchange -> {
            exchange.sendResponseHeaders(200, -1);
            exchange.close();
        });
        testServer.setExecutor(Executors.newSingleThreadExecutor());
        testServer.start();
        reachableUrl = "http://localhost:" + testServer.getAddress().getPort() + "/ok";

        checkExecutor = Executors.newFixedThreadPool(2);
    }

    @AfterEach
    void tearDown() {
        testServer.stop(0);
        checkExecutor.shutdownNow();
    }

    @Test
    void triggerManualCheck_eventuallyMarksReachableUrlAsHealthy() {
        LocalityRepository repository = mock(LocalityRepository.class);

        Locality locality = new Locality();
        locality.setId(1L);
        locality.setName("Test Area");
        locality.setSourceUrl(reachableUrl);

        when(repository.findAll()).thenReturn(List.of(locality));

        SourceLinkHealthChecker checker = new SourceLinkHealthChecker(repository, checkExecutor);

        checker.triggerManualCheck(); // returns immediately -- the actual HTTP check happens after this line

        // Asserting here directly, with no wait, would very likely fail: the background check
        // (an HTTP round-trip plus a repository.save() call) hasn't necessarily happened yet.
        await().atMost(Duration.ofSeconds(5)).untilAsserted(() ->
                verify(repository).save(argThat(saved ->
                        Boolean.TRUE.equals(saved.getSourceUrlHealthy())
                                && saved.getSourceUrlLastChecked() != null))
        );
    }

    @Test
    void triggerManualCheck_marksUnreachableUrlAsUnhealthy() {
        LocalityRepository repository = mock(LocalityRepository.class);

        Locality locality = new Locality();
        locality.setId(2L);
        locality.setName("Broken Link Area");
        // Port 1 is reserved and essentially guaranteed to refuse the connection immediately --
        // deterministic "this will fail" URL, no network flakiness.
        locality.setSourceUrl("http://localhost:1/unreachable");

        when(repository.findAll()).thenReturn(List.of(locality));

        SourceLinkHealthChecker checker = new SourceLinkHealthChecker(repository, checkExecutor);

        checker.triggerManualCheck();

        await().atMost(Duration.ofSeconds(5)).untilAsserted(() ->
                verify(repository).save(argThat(saved ->
                        Boolean.FALSE.equals(saved.getSourceUrlHealthy())))
        );
    }
}
