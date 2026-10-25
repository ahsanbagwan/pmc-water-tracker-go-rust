package org.punewatertracker.config;

import org.junit.jupiter.api.MethodOrderer;
import org.junit.jupiter.api.Order;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.TestMethodOrder;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.context.SpringBootTest;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;
import org.springframework.test.annotation.DirtiesContext;

import static org.assertj.core.api.Assertions.assertThat;

/**
 * Demonstrates Spring's ApplicationContext caching and @DirtiesContext's effect on it, using a
 * minimal inline config (no DB, no web server, no Testcontainers) -- real Spring context
 * machinery, but fast, since this app's real context takes ~20s to build (see the chaos-monkey
 * startup logs) and that cost isn't the point of this demonstration.
 *
 * Methods run in declared order (@TestMethodOrder) so the story reads top to bottom: normal
 * reuse first, then @DirtiesContext breaking that reuse on purpose.
 */
@SpringBootTest(classes = ApplicationContextCachingTest.MinimalConfig.class)
@TestMethodOrder(MethodOrderer.OrderAnnotation.class)
class ApplicationContextCachingTest {

    @Configuration
    static class MinimalConfig {
        @Bean
        public MutableCounter mutableCounter() {
            return new MutableCounter();
        }
    }

    /** A deliberately mutable singleton bean -- stands in for "shared state a test can dirty." */
    static class MutableCounter {
        private int value = 0;

        void increment() {
            value++;
        }

        int get() {
            return value;
        }
    }

    @Autowired
    private MutableCounter counter;

    @Test
    @Order(1)
    void firstTest_mutatesTheSharedBean() {
        counter.increment();
        assertThat(counter.get()).isEqualTo(1);
    }

    @Test
    @Order(2)
    void secondTest_seesThePreviousTestsMutation() {
        // No @DirtiesContext on the previous test -- Spring reused the cached context, which
        // means @Autowired handed us the EXACT SAME MutableCounter instance test 1 mutated.
        // This is context caching's real, sometimes-surprising consequence: state can leak
        // between tests that share a context, unless something is done to prevent it.
        assertThat(counter.get())
                .as("Without @DirtiesContext, the previous test's mutation should have leaked "
                        + "into this one -- same bean instance, same context")
                .isEqualTo(1);

        counter.increment();
        assertThat(counter.get()).isEqualTo(2);
    }

    @Test
    @Order(3)
    @DirtiesContext
    void thirdTest_alsoSeesPriorStateButMarksTheContextDirtyAfterward() {
        // Still the same cached context AT THE START of this test -- @DirtiesContext only
        // takes effect AFTER this method completes, not before it.
        assertThat(counter.get()).isEqualTo(2);

        // After this test method returns, Spring discards the cached context instead of
        // reusing it for whatever runs next.
    }

    @Test
    @Order(4)
    void fourthTest_getsAFreshContextAndAFreshCounter() {
        // Because test 3 was @DirtiesContext, Spring rebuilt the context before this test --
        // @Autowired now hands us a BRAND NEW MutableCounter, back at its initial state,
        // proving the previous tests' mutations did NOT carry over this time.
        assertThat(counter.get())
                .as("After @DirtiesContext forced a rebuild, this should be a fresh bean "
                        + "instance with no memory of the earlier tests' mutations")
                .isEqualTo(0);
    }
}
