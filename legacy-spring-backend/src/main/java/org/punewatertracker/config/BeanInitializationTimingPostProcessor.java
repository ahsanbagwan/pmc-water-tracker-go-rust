package org.punewatertracker.config;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.beans.BeansException;
import org.springframework.beans.factory.config.BeanPostProcessor;
import org.springframework.boot.autoconfigure.condition.ConditionalOnProperty;
import org.springframework.context.ApplicationListener;
import org.springframework.context.event.ContextRefreshedEvent;
import org.springframework.stereotype.Component;

import java.util.Comparator;
import java.util.Map;
import java.util.concurrent.ConcurrentHashMap;

/**
 * Times every bean's INITIALIZATION phase specifically -- not construction, not dependency
 * injection (those happen before postProcessBeforeInitialization is ever called), just the
 * work done by @PostConstruct methods, afterPropertiesSet(), and custom init-methods.
 *
 * Built to help diagnose this app's real, measured ~21s startup time (see
 * "Started PmcWaterTrackerApplication in 20.953 seconds" in the chaos-monkey test logs
 * elsewhere in this project). Two real @PostConstruct methods in this codebase are the
 * specific suspects worth checking against this tool's output:
 *  - WardBoundaryService.loadBoundaries() -- parses a GeoJSON file
 *  - JwtService.init() -- generates a fresh RSA keypair when unconfigured in dev (RSA key
 *    GENERATION, unlike loading a stored key, is genuinely CPU-slow)
 *
 * Every bean in the context goes through both postProcess methods below -- this is in fact
 * the SAME underlying mechanism Spring's own @Cacheable/@Async proxying uses (an
 * AutoProxyCreator is itself a BeanPostProcessor), and the same mechanism Chaos Monkey's own
 * instrumentation uses (visible in the "chaosMonkeyBeanPostProcessor" warnings from the
 * chaos-monkey startup logs) -- this class just does something much simpler: time-and-log,
 * not wrap-and-proxy.
 *
 * Disabled by default -- set app.debug-bean-timing=true to enable, same "opt-in diagnostic,
 * not a default" pattern as PropertyPrecedenceDebugger elsewhere in this project.
 */
@Component
@ConditionalOnProperty(name = "app.debug-bean-timing", havingValue = "true")
public class BeanInitializationTimingPostProcessor implements BeanPostProcessor, ApplicationListener<ContextRefreshedEvent> {

    private static final Logger log = LoggerFactory.getLogger(BeanInitializationTimingPostProcessor.class);
    private static final int TOP_N_TO_LOG = 15;

    private final Map<String, Long> startTimesNanos = new ConcurrentHashMap<>();
    private final Map<String, Long> durationsMillis = new ConcurrentHashMap<>();

    @Override
    public Object postProcessBeforeInitialization(Object bean, String beanName) throws BeansException {
        startTimesNanos.put(beanName, System.nanoTime());
        // Must return the bean itself (or a deliberate replacement) -- returning null here
        // would remove this bean from the application context entirely.
        return bean;
    }

    @Override
    public Object postProcessAfterInitialization(Object bean, String beanName) throws BeansException {
        Long start = startTimesNanos.remove(beanName);
        if (start != null) {
            long durationMs = (System.nanoTime() - start) / 1_000_000;
            durationsMillis.put(beanName, durationMs);
        }
        return bean;
    }

    /** Fires once the whole application context has finished initializing every bean --
     *  logging here (rather than e.g. @PreDestroy) means this shows up immediately at
     *  startup, right where it's actually useful for diagnosing startup time. */
    @Override
    public void onApplicationEvent(ContextRefreshedEvent event) {
        log.info("=== Slowest {} bean initializations (init-phase only, not construction/wiring) ===", TOP_N_TO_LOG);
        durationsMillis.entrySet().stream()
                .sorted(Map.Entry.<String, Long>comparingByValue().reversed())
                .limit(TOP_N_TO_LOG)
                .forEach(e -> log.info("  {} -- {}ms", e.getKey(), e.getValue()));

        long totalMs = durationsMillis.values().stream().mapToLong(Long::longValue).sum();
        log.info("=== Total init-phase time across {} beans: {}ms ===", durationsMillis.size(), totalMs);
    }
}
