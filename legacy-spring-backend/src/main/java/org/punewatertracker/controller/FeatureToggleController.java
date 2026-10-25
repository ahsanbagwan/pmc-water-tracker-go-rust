package org.punewatertracker.controller;

import org.punewatertracker.audit.Audited;
import org.punewatertracker.service.OllamaReportAnalyzer;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.PostMapping;
import org.springframework.web.bind.annotation.RequestMapping;
import org.springframework.web.bind.annotation.RestController;

import java.util.Map;

/**
 * Runtime feature toggles -- flip a feature on/off without a redeploy. Currently just Ollama
 * classification, but shaped to add more toggles the same way later if needed.
 *
 * State is in-memory only (see OllamaReportAnalyzer's AtomicBoolean) -- resets to whatever
 * OLLAMA_ENABLED says on the next restart, same "not persisted" choice Chaos Monkey's own
 * actuator toggle already makes elsewhere in this project. If you need a toggle that survives
 * a restart, that'd mean persisting it (a DB row, or an external flag service) instead of an
 * in-memory AtomicBoolean -- a real, separate step up in complexity from what's here.
 */
@RestController
@RequestMapping("/api/admin/features")
public class FeatureToggleController {

    private final OllamaReportAnalyzer ollamaReportAnalyzer;

    public FeatureToggleController(OllamaReportAnalyzer ollamaReportAnalyzer) {
        this.ollamaReportAnalyzer = ollamaReportAnalyzer;
    }

    @GetMapping("/ollama")
    public Map<String, Boolean> ollamaStatus() {
        return Map.of("enabled", ollamaReportAnalyzer.isEnabled());
    }

    @Audited("TOGGLE_OLLAMA_FEATURE")
    @PostMapping("/ollama/enable")
    public Map<String, Boolean> enableOllama() {
        return Map.of("enabled", ollamaReportAnalyzer.setEnabled(true));
    }

    @Audited("TOGGLE_OLLAMA_FEATURE")
    @PostMapping("/ollama/disable")
    public Map<String, Boolean> disableOllama() {
        return Map.of("enabled", ollamaReportAnalyzer.setEnabled(false));
    }
}
