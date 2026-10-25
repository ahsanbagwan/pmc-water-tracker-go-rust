package org.punewatertracker.config;

import net.javacrumbs.shedlock.core.LockProvider;
import net.javacrumbs.shedlock.provider.jdbctemplate.JdbcTemplateLockProvider;
import net.javacrumbs.shedlock.spring.annotation.EnableSchedulerLock;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;

import javax.sql.DataSource;

/**
 * Without this, @Scheduled runs independently inside EVERY pod's own Spring context -- with
 * 2 replicas, SourceLinkHealthChecker's weekly check would run twice, fully redundantly, from
 * two different pods hitting every locality's source URL simultaneously. ShedLock adds a
 * distributed lock (one row in a shared MySQL table) so only whichever pod acquires it first
 * actually executes; the others see the lock is held and skip that run entirely.
 *
 * defaultLockAtMostFor is a safety net, not the normal case: if a pod died mid-task without
 * releasing the lock, this forces it free again after this duration rather than leaving the
 * job permanently stuck. Set comfortably longer than the job should ever realistically take.
 */
@Configuration
@EnableSchedulerLock(defaultLockAtMostFor = "30m")
public class SchedulerLockConfig {

    @Bean
    public LockProvider lockProvider(DataSource dataSource) {
        return new JdbcTemplateLockProvider(dataSource);
    }
}
