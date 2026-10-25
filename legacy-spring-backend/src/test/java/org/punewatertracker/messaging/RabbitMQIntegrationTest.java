package org.punewatertracker.messaging;

import org.junit.jupiter.api.Test;
import org.springframework.amqp.core.Binding;
import org.springframework.amqp.core.BindingBuilder;
import org.springframework.amqp.core.Queue;
import org.springframework.amqp.core.TopicExchange;
import org.springframework.amqp.rabbit.core.RabbitTemplate;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.context.SpringBootTest;
import org.springframework.boot.test.context.TestConfiguration;
import org.springframework.context.annotation.Bean;
import org.springframework.test.context.DynamicPropertyRegistry;
import org.springframework.test.context.DynamicPropertySource;
import org.testcontainers.containers.RabbitMQContainer;
import org.testcontainers.junit.jupiter.Container;
import org.testcontainers.junit.jupiter.Testcontainers;

import java.time.Instant;

import static org.assertj.core.api.Assertions.assertThat;

/**
 * Requires Docker running locally (or in CI) -- Testcontainers starts a real RabbitMQ broker
 * for this test rather than mocking RabbitTemplate.
 *
 * Uses its own dedicated queue bound to RabbitMQConfig's exchange/routing key, rather than
 * reading from the real report-submitted-queue directly -- that queue also has
 * ReportNotificationListener actively consuming from it in this same test context (since
 * app.rabbitmq.enabled=true activates it too), and RabbitMQ delivers each message on a queue
 * to only one competing consumer. A topic exchange, however, delivers a separate copy of the
 * message to every queue whose binding matches -- so binding a second, test-only queue to the
 * same exchange/routing key gets us a deterministic, race-free copy to assert against.
 */
@Testcontainers
@SpringBootTest(properties = {
        "app.rabbitmq.enabled=true",
        "spring.profiles.active=dev" // H2 for the DB side; only RabbitMQ needs the container here
})
class RabbitMQIntegrationTest {

    @Container
    static RabbitMQContainer rabbitMQContainer = new RabbitMQContainer("rabbitmq:3.13-management-alpine");

    @DynamicPropertySource
    static void rabbitProperties(DynamicPropertyRegistry registry) {
        registry.add("spring.rabbitmq.host", rabbitMQContainer::getHost);
        registry.add("spring.rabbitmq.port", rabbitMQContainer::getAmqpPort);
        registry.add("spring.rabbitmq.username", rabbitMQContainer::getAdminUsername);
        registry.add("spring.rabbitmq.password", rabbitMQContainer::getAdminPassword);
    }

    private static final String TEST_QUEUE = "report-submitted-test-observer-queue";

    @TestConfiguration
    static class TestQueueConfig {
        @Bean
        Queue testObserverQueue() {
            return new Queue(TEST_QUEUE, false, false, true); // non-durable, auto-delete
        }

        @Bean
        Binding testObserverBinding(Queue testObserverQueue, TopicExchange reportExchange) {
            return BindingBuilder.bind(testObserverQueue)
                    .to(reportExchange)
                    .with(RabbitMQConfig.REPORT_SUBMITTED_ROUTING_KEY);
        }
    }

    @Autowired
    private ReportEventPublisher reportEventPublisher;

    @Autowired
    private RabbitTemplate rabbitTemplate;

    @Test
    void publishReportSubmitted_deliversMessageToBoundQueue() {
        ReportSubmittedEvent event = new ReportSubmittedEvent(
                123L, "Test Locality", "TANKER_DEPENDENT", "integration test", Instant.now());

        reportEventPublisher.publishReportSubmitted(event);

        // Blocks up to 5s waiting for the message rather than polling -- simpler than a
        // manual retry loop for this single assertion.
        Object received = rabbitTemplate.receiveAndConvert(TEST_QUEUE, 5000);

        assertThat(received).isInstanceOf(ReportSubmittedEvent.class);
        ReportSubmittedEvent receivedEvent = (ReportSubmittedEvent) received;
        assertThat(receivedEvent.localityId()).isEqualTo(123L);
        assertThat(receivedEvent.localityName()).isEqualTo("Test Locality");
        assertThat(receivedEvent.proposedStatus()).isEqualTo("TANKER_DEPENDENT");
    }
}
