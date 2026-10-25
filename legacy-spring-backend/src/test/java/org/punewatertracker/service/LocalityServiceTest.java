package org.punewatertracker.service;

import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.mockito.ArgumentCaptor;
import org.mockito.MockitoAnnotations;
import org.mockito.Mock;
import org.punewatertracker.messaging.ReportEventPublisher;
import org.punewatertracker.messaging.ReportSubmittedEvent;
import org.punewatertracker.model.Locality;
import org.punewatertracker.model.ReportUrgency;
import org.punewatertracker.model.WaterStatus;
import org.punewatertracker.repository.LocalityRepository;

import java.time.LocalDate;
import java.util.List;
import java.util.NoSuchElementException;
import java.util.Optional;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;
import static org.mockito.ArgumentMatchers.any;
import static org.mockito.Mockito.never;
import static org.mockito.Mockito.verify;
import static org.mockito.Mockito.when;

class LocalityServiceTest {

    @Mock
    private LocalityRepository repository;

    @Mock
    private ReportEventPublisher reportEventPublisher;

    @Mock
    private NearbyReportFinder nearbyReportFinder;

    @Mock
    private OllamaReportAnalyzer ollamaReportAnalyzer;

    private LocalityService service;

    @BeforeEach
    void setUp() {
        MockitoAnnotations.openMocks(this);
        service = new LocalityService(repository, reportEventPublisher, nearbyReportFinder, ollamaReportAnalyzer);
    }

    @Test
    void createVerified_setsVerifiedTrueAndDefaultsLastVerified() {
        Locality input = new Locality();
        input.setName("Test Area");
        input.setStatus(WaterStatus.MUNICIPAL);
        input.setLatitude(18.5);
        input.setLongitude(73.8);

        when(repository.save(any(Locality.class))).thenAnswer(inv -> inv.getArgument(0));

        Locality result = service.createVerified(input);

        assertThat(result.isVerified()).isTrue();
        assertThat(result.getLastVerified()).isEqualTo(LocalDate.now());
        verify(repository).save(input);
        verify(reportEventPublisher, never()).publishReportSubmitted(any());
    }

    @Test
    void createVerified_preservesExplicitLastVerifiedDate() {
        Locality input = new Locality();
        input.setLastVerified(LocalDate.of(2026, 1, 1));
        when(repository.save(any(Locality.class))).thenAnswer(inv -> inv.getArgument(0));

        Locality result = service.createVerified(input);

        assertThat(result.getLastVerified()).isEqualTo(LocalDate.of(2026, 1, 1));
    }

    @Test
    void createUnverifiedReport_setsUnverifiedAndPublishesEvent() {
        Locality input = new Locality();
        input.setName("Reported Area");
        input.setStatus(WaterStatus.TANKER_DEPENDENT);

        Locality saved = new Locality();
        saved.setId(42L);
        saved.setName("Reported Area");
        saved.setStatus(WaterStatus.TANKER_DEPENDENT);
        saved.setVerified(false);

        when(repository.save(any(Locality.class))).thenReturn(saved);

        Locality result = service.createUnverifiedReport(input);

        assertThat(result.isVerified()).isFalse();
        assertThat(input.isVerified()).isFalse();
        assertThat(input.getId()).isNull(); // id was cleared before save, regardless of caller-supplied value

        ArgumentCaptor<ReportSubmittedEvent> captor = ArgumentCaptor.forClass(ReportSubmittedEvent.class);
        verify(reportEventPublisher).publishReportSubmitted(captor.capture());
        assertThat(captor.getValue().localityId()).isEqualTo(42L);
        assertThat(captor.getValue().localityName()).isEqualTo("Reported Area");
        assertThat(captor.getValue().proposedStatus()).isEqualTo("TANKER_DEPENDENT");
    }

    @Test
    void findVisible_withStatus_excludesUnverifiedEntries() {
        Locality verified = new Locality();
        verified.setVerified(true);
        Locality unverified = new Locality();
        unverified.setVerified(false);

        when(repository.findByStatus(WaterStatus.MUNICIPAL)).thenReturn(List.of(verified, unverified));

        List<Locality> result = service.findVisible(WaterStatus.MUNICIPAL, null);

        assertThat(result).containsExactly(verified);
    }

    @Test
    void findVisible_withSearch_delegatesToRepository() {
        when(repository.findByNameContainingIgnoreCaseAndVerifiedTrue("wagholi"))
                .thenReturn(List.of(new Locality()));

        List<Locality> result = service.findVisible(null, "wagholi");

        assertThat(result).hasSize(1);
        verify(repository).findByNameContainingIgnoreCaseAndVerifiedTrue("wagholi");
    }

    @Test
    void findVisible_noFilters_returnsAllVerified() {
        when(repository.findByVerifiedTrue()).thenReturn(List.of(new Locality(), new Locality()));

        List<Locality> result = service.findVisible(null, null);

        assertThat(result).hasSize(2);
    }

    @Test
    void findVisible_statusTakesPriorityOverSearch() {
        when(repository.findByStatus(WaterStatus.MIXED)).thenReturn(List.of());

        service.findVisible(WaterStatus.MIXED, "some search text");

        verify(repository).findByStatus(WaterStatus.MIXED);
        verify(repository, never()).findByNameContainingIgnoreCaseAndVerifiedTrue(any());
    }

    @Test
    void findById_notFound_throwsNoSuchElementException() {
        when(repository.findById(99L)).thenReturn(Optional.empty());

        assertThatThrownBy(() -> service.findById(99L))
                .isInstanceOf(NoSuchElementException.class)
                .hasMessageContaining("99");
    }

    @Test
    void approveReport_setsVerifiedTrue() {
        Locality unverified = new Locality();
        unverified.setId(7L);
        unverified.setVerified(false);

        when(repository.findById(7L)).thenReturn(Optional.of(unverified));
        when(repository.save(any(Locality.class))).thenAnswer(inv -> inv.getArgument(0));

        Locality result = service.approveReport(7L);

        assertThat(result.isVerified()).isTrue();
    }

    @Test
    void update_appliesAllPatchFieldsOntoExistingEntity() {
        Locality existing = new Locality();
        existing.setId(3L);
        existing.setName("Old Name");
        existing.setStatus(WaterStatus.TANKER_DEPENDENT);

        Locality patch = new Locality();
        patch.setName("New Name");
        patch.setWard("Ward 7");
        patch.setStatus(WaterStatus.MUNICIPAL);
        patch.setLatitude(18.1);
        patch.setLongitude(73.1);
        patch.setNotes("updated notes");
        patch.setSourceName("Source");
        patch.setSourceUrl("https://example.com");
        patch.setLastVerified(LocalDate.of(2026, 6, 1));

        when(repository.findById(3L)).thenReturn(Optional.of(existing));
        when(repository.save(any(Locality.class))).thenAnswer(inv -> inv.getArgument(0));

        Locality result = service.update(3L, patch);

        assertThat(result.getName()).isEqualTo("New Name");
        assertThat(result.getWard()).isEqualTo("Ward 7");
        assertThat(result.getStatus()).isEqualTo(WaterStatus.MUNICIPAL);
        assertThat(result.getLastVerified()).isEqualTo(LocalDate.of(2026, 6, 1));
    }

    @Test
    void update_missingLastVerifiedOnPatch_defaultsToToday() {
        Locality existing = new Locality();
        existing.setId(3L);

        Locality patch = new Locality();
        patch.setLastVerified(null);

        when(repository.findById(3L)).thenReturn(Optional.of(existing));
        when(repository.save(any(Locality.class))).thenAnswer(inv -> inv.getArgument(0));

        Locality result = service.update(3L, patch);

        assertThat(result.getLastVerified()).isEqualTo(LocalDate.now());
    }

    @Test
    void createUnverifiedReport_withDescription_appliesOllamaUrgencyClassification() {
        Locality input = new Locality();
        input.setName("Test Area");
        input.setCitizenDescription("Pipeline burst near the main road");

        when(repository.save(any(Locality.class))).thenAnswer(inv -> inv.getArgument(0));
        when(nearbyReportFinder.findClosestCandidate(input)).thenReturn(null); // no nearby candidate
        when(ollamaReportAnalyzer.analyze("Pipeline burst near the main road", null))
                .thenReturn(new OllamaReportAnalyzer.Analysis(
                        ReportUrgency.HIGH, 80, "Mentions a burst pipeline.", false, null, null));

        Locality result = service.createUnverifiedReport(input);

        assertThat(result.getReportedUrgency()).isEqualTo(ReportUrgency.HIGH);
        assertThat(result.getUrgencyScore()).isEqualTo(80);
        assertThat(result.getUrgencySignals()).isEqualTo("Mentions a burst pipeline.");
    }

    @Test
    void createUnverifiedReport_ollamaUnreachable_leavesUrgencyNullWithoutThrowing() {
        Locality input = new Locality();
        input.setName("Test Area");
        input.setCitizenDescription("something went wrong");

        when(repository.save(any(Locality.class))).thenAnswer(inv -> inv.getArgument(0));
        when(nearbyReportFinder.findClosestCandidate(input)).thenReturn(null);
        when(ollamaReportAnalyzer.analyze(any(), any())).thenReturn(null); // simulates Ollama being disabled/unreachable

        Locality result = service.createUnverifiedReport(input);

        assertThat(result.getReportedUrgency()).isNull();
    }

    @Test
    void createUnverifiedReport_explicitUrgencyProvided_notOverwrittenByOllama() {
        Locality input = new Locality();
        input.setName("Test Area");
        input.setCitizenDescription("No water at all");
        input.setReportedUrgency(ReportUrgency.LOW); // caller/admin already set this explicitly

        when(repository.save(any(Locality.class))).thenAnswer(inv -> inv.getArgument(0));
        when(nearbyReportFinder.findClosestCandidate(input)).thenReturn(null);
        // Unlike the old classifier, Ollama analysis still runs even when urgency is already
        // set -- it also handles duplicate detection, which is independent of urgency. Only
        // the urgency FIELDS should be left alone; the analyze() call itself isn't skipped.
        when(ollamaReportAnalyzer.analyze(any(), any()))
                .thenReturn(new OllamaReportAnalyzer.Analysis(
                        ReportUrgency.CRITICAL, 95, "would have said critical", false, null, null));

        Locality result = service.createUnverifiedReport(input);

        assertThat(result.getReportedUrgency())
                .as("The explicitly-set urgency should win over whatever Ollama would have said")
                .isEqualTo(ReportUrgency.LOW);
    }

    @Test
    void createUnverifiedReport_nearbyDuplicateFound_setsPossibleDuplicateFields() {
        Locality candidate = new Locality();
        candidate.setId(11L);
        candidate.setCitizenDescription("Same burst, different reporter");

        Locality input = new Locality();
        input.setName("Test Area");
        input.setCitizenDescription("Pipeline burst near ABC Chowk");

        when(repository.save(any(Locality.class))).thenAnswer(inv -> inv.getArgument(0));
        when(nearbyReportFinder.findClosestCandidate(input)).thenReturn(candidate);
        when(ollamaReportAnalyzer.analyze("Pipeline burst near ABC Chowk", "Same burst, different reporter"))
                .thenReturn(new OllamaReportAnalyzer.Analysis(
                        ReportUrgency.HIGH, 80, "burst pipeline", true, 0.9, "same incident"));

        Locality result = service.createUnverifiedReport(input);

        assertThat(result.getPossibleDuplicateOfId()).isEqualTo(11L);
        assertThat(result.getDuplicateConfidence()).isEqualTo(0.9);
    }

    @Test
    void delete_callsRepositoryDeleteById() {
        service.delete(5L);
        verify(repository).deleteById(5L);
    }
}
