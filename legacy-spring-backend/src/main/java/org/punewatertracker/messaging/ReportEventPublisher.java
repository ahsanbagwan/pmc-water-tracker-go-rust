package org.punewatertracker.messaging;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.amqp.rabbit.core.RabbitTemplate;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.stereotype.Service;

@Service
public class ReportEventPublisher {

    private static final Logger log = LoggerFactory.getLogger(ReportEventPublisher.class);

    @Value("${app.rabbitmq.enabled:false}")
    private boolean enabled;

    private final RabbitTemplate rabbitTemplate;

    public ReportEventPublisher(RabbitTemplate rabbitTemplate) {
        this.rabbitTemplate = rabbitTemplate;
    }

    /**
     * Fire-and-forget: report submission has already succeeded (saved to the DB) by the time
     * this is called, so a disabled/unreachable broker never blocks or fails the citizen's
     * request -- it just means no async notification goes out.
     */
    public void publishReportSubmitted(ReportSubmittedEvent event) {
        if (!enabled) {
            return;
        }
        try {
            rabbitTemplate.convertAndSend(RabbitMQConfig.EXCHANGE, RabbitMQConfig.REPORT_SUBMITTED_ROUTING_KEY, event);
        } catch (Exception ex) {
            // Without this catch, an unreachable (not just disabled) broker would propagate
            // an exception back up through LocalityService and potentially fail the citizen's
            // report submission -- directly contradicting the guarantee described above. The
            // report itself is already saved by this point; only the notification is at risk.
            log.warn("Failed to publish report-submitted event for locality '{}': {}. "
                    + "The report itself was already saved successfully.",
                    event.localityName(), ex.getMessage());
        }
    }
}
