package org.punewatertracker.benchmark;

import org.openjdk.jmh.annotations.*;
import org.springframework.web.client.RestTemplate;

import java.util.List;
import java.util.concurrent.*;
import java.util.stream.Collectors;
import java.util.stream.IntStream;

@State(Scope.Thread)
@BenchmarkMode(Mode.Throughput)
@OutputTimeUnit(java.util.concurrent.TimeUnit.SECONDS)
@Warmup(iterations = 3, time = 5, timeUnit = java.util.concurrent.TimeUnit.SECONDS)
@Measurement(iterations = 5, time = 5, timeUnit = java.util.concurrent.TimeUnit.SECONDS)
@Fork(1)
public class LinkCheckerBenchmark {

    private ExecutorService executor;
    private RestTemplate restTemplate;
    private List<String> urls;

    @Setup
    public void setup() {
        // Simulate Spring's ThreadPoolTaskExecutor (Core=100, Max=100, Queue=10000)
        executor = new ThreadPoolExecutor(
                100, 100, 0L, TimeUnit.MILLISECONDS,
                new LinkedBlockingQueue<>(10000)
        );

        restTemplate = new RestTemplate();

        // Generate 1,000 dummy URLs (using httpbin for consistent, fast 200 OK responses)
        // Note: For absolute zero-network benchmarks, you would use WireMock here instead.
        urls = IntStream.range(0, 1000)
                .mapToObj(i -> "http://httpbin.org/status/200")
                .collect(Collectors.toList());
    }

    @TearDown
    public void tearDown() {
        executor.shutdownNow();
    }

    @Benchmark
    public List<Boolean> benchmarkSpringExecutor() {
        List<CompletableFuture<Boolean>> futures = urls.stream()
                .map(url -> CompletableFuture.supplyAsync(() -> {
                    try {
                        // Simulate the HTTP check
                        restTemplate.getForObject(url, String.class);
                        return true;
                    } catch (Exception e) {
                        return false;
                    }
                }, executor))
                .collect(Collectors.toList());

        // Block until all futures complete (simulating the HTTP request waiting for the batch)
        return futures.stream()
                .map(CompletableFuture::join)
                .collect(Collectors.toList());
    }
}
