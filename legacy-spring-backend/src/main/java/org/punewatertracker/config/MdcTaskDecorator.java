package org.punewatertracker.config;

import org.slf4j.MDC;
import org.springframework.core.task.TaskDecorator;

import java.util.Map;

/**
 * ThreadLocal-backed MDC doesn't propagate onto thread-pool worker threads automatically --
 * pool threads are pre-existing, reused threads, not children of whatever thread submitted
 * the work, so there's no parent-child relationship for a ThreadLocal to inherit through.
 *
 * TaskDecorator.decorate() runs on the SUBMITTING thread, synchronously, at the moment a task
 * is handed to the executor -- so capturing MDC here genuinely captures the submitter's real
 * context. The returned Runnable is what actually executes later, on a worker thread.
 *
 * Restoring the worker thread's own previous MDC state afterward (not just clearing it) matters
 * because worker threads are reused across many unrelated tasks over the executor's lifetime --
 * skipping that step would leak one task's correlation ID into the next task that same worker
 * thread happens to pick up.
 */
public class MdcTaskDecorator implements TaskDecorator {

    @Override
    public Runnable decorate(Runnable runnable) {
        Map<String, String> submitterContext = MDC.getCopyOfContextMap();

        return () -> {
            Map<String, String> workerThreadPreviousContext = MDC.getCopyOfContextMap();
            try {
                if (submitterContext != null) {
                    MDC.setContextMap(submitterContext);
                } else {
                    MDC.clear();
                }
                runnable.run();
            } finally {
                if (workerThreadPreviousContext != null) {
                    MDC.setContextMap(workerThreadPreviousContext);
                } else {
                    MDC.clear();
                }
            }
        };
    }
}
