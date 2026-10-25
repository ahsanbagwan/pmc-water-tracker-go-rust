package org.punewatertracker.config;

import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;
import org.springframework.scheduling.annotation.EnableAsync;
import org.springframework.scheduling.annotation.EnableScheduling;
import org.springframework.scheduling.concurrent.ThreadPoolTaskExecutor;

import java.util.concurrent.Executor;

@Configuration
@EnableAsync
@EnableScheduling
public class TaskExecutorConfig {

    /**
     * Deliberately separate from Spring MVC's own request-handling threads (Tomcat's pool) --
     * a slow batch of link checks can never starve request handling, or vice versa. Sized for
     * I/O-bound work (blocked waiting on network responses, not CPU-bound computation), so a
     * pool larger than the core count is appropriate here.
     */
    @Bean(name = "sourceLinkCheckExecutor")
    public Executor sourceLinkCheckExecutor() {
        ThreadPoolTaskExecutor executor = new ThreadPoolTaskExecutor();
        executor.setCorePoolSize(4);
        executor.setMaxPoolSize(8);
        executor.setQueueCapacity(100);
        executor.setThreadNamePrefix("link-check-");
        executor.setTaskDecorator(new MdcTaskDecorator());
        executor.initialize();
        return executor;
    }
}
