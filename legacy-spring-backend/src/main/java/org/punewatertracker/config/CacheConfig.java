package org.punewatertracker.config;

import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;
import org.springframework.data.redis.cache.RedisCacheConfiguration;
import org.springframework.data.redis.cache.RedisCacheManager;
import org.springframework.data.redis.connection.RedisConnectionFactory;
import org.springframework.cache.annotation.EnableCaching;

import java.time.Duration;

/**
 * Redis-backed, not the original in-memory Caffeine -- this is the exact "revisit this if
 * that ever changes" the original CacheConfig comment flagged. With multiple K8s replicas,
 * each pod having its own separate in-memory cache meant an @CacheEvict on one pod (say, an
 * admin's update) was invisible to every OTHER pod, which kept serving stale data from their
 * own local cache for up to the full TTL. Reusing the SAME Redis instance already provisioned
 * for rate limiting and the Ollama feature toggle -- every pod now reads/writes ONE shared
 * cache, so an eviction on any pod is immediately visible to all of them.
 *
 * @Cacheable/@CacheEvict on LocalityService needed ZERO changes -- both Caffeine and Redis
 * implementations satisfy the same Spring CacheManager interface, so only this bean changes.
 */
@Configuration
@EnableCaching
public class CacheConfig {

    public static final String LOCALITIES_CACHE = "localities";

    @Bean
    public RedisCacheManager cacheManager(RedisConnectionFactory connectionFactory) {
        RedisCacheConfiguration config = RedisCacheConfiguration.defaultCacheConfig()
                .entryTtl(Duration.ofSeconds(30)); // same TTL as the original Caffeine config

        return RedisCacheManager.builder(connectionFactory)
                .cacheDefaults(config)
                .build();
    }
}
