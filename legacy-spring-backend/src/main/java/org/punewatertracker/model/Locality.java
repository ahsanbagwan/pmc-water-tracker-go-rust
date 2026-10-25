package org.punewatertracker.model;

import jakarta.persistence.*;
import jakarta.validation.constraints.NotBlank;
import jakarta.validation.constraints.NotNull;
import java.time.Instant;
import java.time.LocalDate;

@Entity
@Table(name = "locality")
public class Locality {

    @Id
    @GeneratedValue(strategy = GenerationType.IDENTITY)
    private Long id;

    @NotBlank
    @Column(nullable = false)
    private String name;

    /** PMC ward name/number the locality falls under, where known. */
    private String ward;

    @NotNull
    @Enumerated(EnumType.STRING)
    @Column(nullable = false)
    private WaterStatus status;

    @NotNull
    private Double latitude;

    @NotNull
    private Double longitude;

    /** Free-text context: what's actually going on, in plain language. */
    @Column(length = 1000)
    private String notes;

    /** Where this status claim comes from — required so entries stay checkable, not asserted. */
    @Column(length = 500)
    private String sourceName;

    @Column(length = 500)
    private String sourceUrl;

    /** Date this entry was last checked/confirmed against a source. */
    private LocalDate lastVerified;

    /** False for citizen-submitted reports until an admin reviews and confirms them. */
    @Column(nullable = false)
    private boolean verified = true;

    /** Null = never checked. Set by SourceLinkHealthChecker. */
    private Boolean sourceUrlHealthy;

    private Instant sourceUrlLastChecked;

    /** Free-text description a citizen typed when submitting a report, e.g. "No water since yesterday". */
    @Column(length = 1000)
    private String citizenDescription;

    /** Suggested by OllamaReportAnalyzer from citizenDescription -- null if never classified
     *  (including when Ollama is disabled/unreachable), or if the citizen/admin already set
     *  it explicitly, in which case it's left alone. */
    @Enumerated(EnumType.STRING)
    private ReportUrgency reportedUrgency;

    /** Model confidence (0-100) behind reportedUrgency. */
    private Integer urgencyScore;

    /** Model's one-sentence reasoning behind reportedUrgency. */
    @Column(length = 500)
    private String urgencySignals;

    /** Set if this report is judged a likely duplicate of an earlier unverified report --
     *  points at that report's id, null if no match found. */
    private Long possibleDuplicateOfId;

    /** Model confidence (0.0-1.0) behind possibleDuplicateOfId. Null if that's null. */
    private Double duplicateConfidence;

    public Locality() {
    }

    public Long getId() {
        return id;
    }

    public void setId(Long id) {
        this.id = id;
    }

    public String getName() {
        return name;
    }

    public void setName(String name) {
        this.name = name;
    }

    public String getWard() {
        return ward;
    }

    public void setWard(String ward) {
        this.ward = ward;
    }

    public WaterStatus getStatus() {
        return status;
    }

    public void setStatus(WaterStatus status) {
        this.status = status;
    }

    public Double getLatitude() {
        return latitude;
    }

    public void setLatitude(Double latitude) {
        this.latitude = latitude;
    }

    public Double getLongitude() {
        return longitude;
    }

    public void setLongitude(Double longitude) {
        this.longitude = longitude;
    }

    public String getNotes() {
        return notes;
    }

    public void setNotes(String notes) {
        this.notes = notes;
    }

    public String getSourceName() {
        return sourceName;
    }

    public void setSourceName(String sourceName) {
        this.sourceName = sourceName;
    }

    public String getSourceUrl() {
        return sourceUrl;
    }

    public void setSourceUrl(String sourceUrl) {
        this.sourceUrl = sourceUrl;
    }

    public LocalDate getLastVerified() {
        return lastVerified;
    }

    public void setLastVerified(LocalDate lastVerified) {
        this.lastVerified = lastVerified;
    }

    public boolean isVerified() {
        return verified;
    }

    public void setVerified(boolean verified) {
        this.verified = verified;
    }

    public Boolean getSourceUrlHealthy() {
        return sourceUrlHealthy;
    }

    public void setSourceUrlHealthy(Boolean sourceUrlHealthy) {
        this.sourceUrlHealthy = sourceUrlHealthy;
    }

    public Instant getSourceUrlLastChecked() {
        return sourceUrlLastChecked;
    }

    public void setSourceUrlLastChecked(Instant sourceUrlLastChecked) {
        this.sourceUrlLastChecked = sourceUrlLastChecked;
    }

    public String getCitizenDescription() {
        return citizenDescription;
    }

    public void setCitizenDescription(String citizenDescription) {
        this.citizenDescription = citizenDescription;
    }

    public ReportUrgency getReportedUrgency() {
        return reportedUrgency;
    }

    public void setReportedUrgency(ReportUrgency reportedUrgency) {
        this.reportedUrgency = reportedUrgency;
    }

    public Integer getUrgencyScore() {
        return urgencyScore;
    }

    public void setUrgencyScore(Integer urgencyScore) {
        this.urgencyScore = urgencyScore;
    }

    public String getUrgencySignals() {
        return urgencySignals;
    }

    public void setUrgencySignals(String urgencySignals) {
        this.urgencySignals = urgencySignals;
    }

    public Long getPossibleDuplicateOfId() {
        return possibleDuplicateOfId;
    }

    public void setPossibleDuplicateOfId(Long possibleDuplicateOfId) {
        this.possibleDuplicateOfId = possibleDuplicateOfId;
    }

    public Double getDuplicateConfidence() {
        return duplicateConfidence;
    }

    public void setDuplicateConfidence(Double duplicateConfidence) {
        this.duplicateConfidence = duplicateConfidence;
    }
}
