package org.punewatertracker.config;

import org.junit.jupiter.api.Test;

import java.util.concurrent.BrokenBarrierException;
import java.util.concurrent.CountDownLatch;
import java.util.concurrent.CyclicBarrier;
import java.util.concurrent.Executor;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.TimeoutException;

import static org.assertj.core.api.Assertions.assertThat;

/**
 * Proves sourceLinkCheckExecutor() genuinely provides parallelism -- not just "it has a pool
 * size configured," but that N tasks can actually be *running at the same instant*, matching
 * its corePoolSize of 4.
 *
 * A CyclicBarrier is the right tool for this specific claim: it can ONLY release once exactly
 * N threads have simultaneously called await() on it. If the executor were secretly serial
 * (e.g. someone later drops corePoolSize to 1 without noticing the concurrency implication),
 * the first task would block on the barrier forever waiting for 3 others that can never start
 * -- turning "the executor stopped being concurrent" into a fast, loud test failure instead of
 * a silent production regression (link checks quietly taking 4x longer, unnoticed).
 *
 * CountDownLatch is used separately, for its more common purpose: letting the main test thread
 * know when all the worker threads have finished, so the test doesn't return before the
 * assertions it depends on are actually meaningful.
 */
class TaskExecutorConfigTest {

    @Test
    void sourceLinkCheckExecutor_runsFourTasksConcurrently() throws Exception {
        Executor executor = new TaskExecutorConfig().sourceLinkCheckExecutor();
        int concurrentTasks = 4; // matches corePoolSize in TaskExecutorConfig

        CyclicBarrier barrier = new CyclicBarrier(concurrentTasks);
        CountDownLatch allTasksFinished = new CountDownLatch(concurrentTasks);

        for (int i = 0; i < concurrentTasks; i++) {
            executor.execute(() -> {
                try {
                    // Only returns once all `concurrentTasks` threads have reached this line
                    // simultaneously -- impossible unless they're genuinely running at once.
                    barrier.await(5, TimeUnit.SECONDS);
                } catch (InterruptedException | BrokenBarrierException | TimeoutException e) {
                    throw new RuntimeException(e);
                } finally {
                    allTasksFinished.countDown();
                }
            });
        }

        boolean completedInTime = allTasksFinished.await(5, TimeUnit.SECONDS);

        assertThat(completedInTime)
                .as("All %d tasks should complete once they meet at the barrier together -- "
                        + "if this times out, the executor isn't actually running them concurrently "
                        + "(e.g. corePoolSize got reduced below %d)", concurrentTasks, concurrentTasks)
                .isTrue();
    }
}
