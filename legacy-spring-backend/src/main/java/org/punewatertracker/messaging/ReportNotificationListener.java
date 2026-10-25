package org.punewatertracker.messaging;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.amqp.rabbit.annotation.RabbitListener;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.boot.autoconfigure.condition.ConditionalOnProperty;
import org.springframework.stereotype.Component;
import org.springframework.web.client.RestClient;

import java.util.List;
import java.util.Map;

/**
 * Only registered when RabbitMQ is enabled -- see RabbitMQConfig.
 *
 * Emails the configured admin address when a citizen submits a report, via Brevo's HTTP API
 * (not SMTP -- Render's free tier blocks outbound SMTP ports entirely, so a normal SMTP
 * provider cannot work there regardless of credentials; Brevo's API goes over HTTPS/443 like
 * everything else, so it isn't affected).
 *
 * If mail isn't configured (missing BREVO_API_KEY, MAIL_FROM_ADDRESS, or
 * ADMIN_NOTIFICATION_EMAIL), or sending fails for any reason, this logs a warning and moves
 * on -- it never throws, since a notification problem should never look like a
 * message-processing failure and trigger RabbitMQ redelivery/retry.
 */
@Component
@ConditionalOnProperty(name = "app.rabbitmq.enabled", havingValue = "true")
public class ReportNotificationListener {

    private static final Logger log = LoggerFactory.getLogger(ReportNotificationListener.class);
    private static final String BREVO_SEND_EMAIL_URL = "https://api.brevo.com/v3/smtp/email";

    private final RestClient restClient = RestClient.create();

    @Value("${app.brevo-api-key:}")
    private String brevoApiKey;

    @Value("${app.mail-from-address:}")
    private String mailFromAddress;

    @Value("${app.admin-notification-email:}")
    private String adminNotificationEmail;

    @RabbitListener(queues = RabbitMQConfig.REPORT_SUBMITTED_QUEUE)
    public void onReportSubmitted(ReportSubmittedEvent event) {
        if (brevoApiKey.isBlank() || mailFromAddress.isBlank() || adminNotificationEmail.isBlank()) {
            log.warn("Skipping email notification (BREVO_API_KEY, MAIL_FROM_ADDRESS, or "
                    + "ADMIN_NOTIFICATION_EMAIL not set). Report on '{}' was still saved -- "
                    + "this only affects the notification.", event.localityName());
            return;
        }

        try {
            String textContent =
                    "A citizen submitted a report for " + event.localityName() + ".\n\n"
                    + "Proposed status: " + event.proposedStatus() + "\n"
                    + "Notes: " + (event.notes() == null || event.notes().isBlank() ? "(none)" : event.notes()) + "\n"
                    + "Submitted at: " + event.submittedAt() + "\n"
                    + "Locality id: " + event.localityId() + "\n\n"
                    + "Review and approve it with:\n"
                    + "POST /api/localities/reports/" + event.localityId() + "/approve";

            Map<String, Object> body = Map.of(
                    "sender", Map.of("email", mailFromAddress),
                    "to", List.of(Map.of("email", adminNotificationEmail)),
                    "subject", "New water status report: " + event.localityName(),
                    "textContent", textContent
            );

            restClient.post()
                    .uri(BREVO_SEND_EMAIL_URL)
                    .header("api-key", brevoApiKey)
                    .header("Content-Type", "application/json")
                    .body(body)
                    .retrieve()
                    .toBodilessEntity();

            log.info("Sent report notification email for '{}'", event.localityName());
        } catch (Exception ex) {
            log.warn("Failed to send email for '{}': {}. The report itself was still saved successfully.",
                    event.localityName(), ex.getMessage());
        }
    }
}
