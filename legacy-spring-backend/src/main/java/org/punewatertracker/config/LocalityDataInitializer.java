package org.punewatertracker.config;

import org.punewatertracker.model.Locality;
import org.punewatertracker.model.WaterStatus;
import org.punewatertracker.repository.LocalityRepository;
import org.springframework.boot.CommandLineRunner;
import org.springframework.stereotype.Component;

import java.time.LocalDate;
import java.util.List;

import static org.punewatertracker.model.WaterStatus.*;

/**
 * Seeds the locality table on first run. Compiled from public reporting as of July 2026 --
 * every row cites where the status claim comes from and when it was last checked. This is a
 * starting point, not a real-time feed; update entries via the API as better info comes in.
 *
 * Runs on every startup and inserts any locality from SEED that isn't already present (matched
 * by name). Safe against a persistent database -- won't duplicate existing rows, and correctly
 * picks up new entries added to SEED later instead of silently skipping them.
 */
@Component
public class LocalityDataInitializer implements CommandLineRunner {

    private final LocalityRepository repository;

    public LocalityDataInitializer(LocalityRepository repository) {
        this.repository = repository;
    }

    @Override
    public void run(String... args) {
        // Insert only what's missing -- safe to run on every restart, and means adding new
        // entries to SEED later actually reaches an already-seeded database (unlike an
        // all-or-nothing "only seed if empty" check, which would silently skip them forever).
        SEED.stream()
                .filter(seed -> !repository.existsByName(seed.name()))
                .forEach(seed -> repository.save(seed.toLocality()));
    }

    private record Seed(String name, String ward, WaterStatus status, double lat, double lng,
                         String notes, String sourceName, String sourceUrl, String lastVerified) {
        Locality toLocality() {
            Locality l = new Locality();
            l.setName(name);
            l.setWard(ward);
            l.setStatus(status);
            l.setLatitude(lat);
            l.setLongitude(lng);
            l.setNotes(notes);
            l.setSourceName(sourceName);
            l.setSourceUrl(sourceUrl);
            l.setLastVerified(LocalDate.parse(lastVerified));
            l.setVerified(true);
            return l;
        }
    }

    private static final String GENERAL_KNOWLEDGE = "General public knowledge (unverified against a specific 2026 report)";
    private static final String BUDGET_2026_27 = "PMC Budget 2026-27 (Free Press Journal)";
    private static final String BUDGET_2026_27_URL = "https://www.freepressjournal.in/pune/pune-budget-2026-27-pmc-to-focus-on-reducing-water-leakage-expanding-supply-to-merged-villages";
    private static final String ELECTIONS_2026 = "Free Press Journal - PMC Elections 2026";
    private static final String ELECTIONS_2026_URL = "https://www.freepressjournal.in/pune/pmc-elections-2026-first-time-voters-from-merged-villages-question-years-of-neglect";
    private static final String BHAMA_ASKHED = "Punekar News / Free Press Journal";
    private static final String BHAMA_ASKHED_URL = "https://www.punekarnews.in/pune-water-supply-schedule-revised-in-viman-nagar-lohgaon-dhanori-areas-under-bhama-askhed-scheme/";
    private static final String REALTY_GUIDE = "Pune Realty Hub - Water Connection Guide 2026";
    private static final String REALTY_GUIDE_URL = "https://punerealtyhub.com/blog/pune-property-water-connection-guide";
    private static final String RTI_TANKER_SPEND = "Pune Pulse - RTI Tanker Expenditure Report";
    private static final String RTI_TANKER_SPEND_URL = "https://www.mypunepulse.com/pune-civic-body-spends-over-%E2%82%B927-crore-on-water-tankers/";
    private static final String SUMMER_TANKER_SURGE = "The Bridge Chronicle - Pune Tanker Raj Returns as Summer Begins";
    private static final String SUMMER_TANKER_SURGE_URL = "https://www.thebridgechronicle.com/pune/pune-tanker-raj-water-demand-summer-agn97";

    private static final List<Seed> SEED = List.of(
            // Long-established core PMC areas -- general baseline, not a specific dated 2026 source
            new Seed("Kothrud", "Kothrud", MUNICIPAL, 18.5074, 73.8077,
                    "Long-established core PMC area with mature piped infrastructure. Not tied to a specific dated 2026 source -- treat as a general baseline and confirm locally if precision matters.",
                    GENERAL_KNOWLEDGE, null, "2026-07-01"),
            new Seed("Aundh", "Aundh-Baner", MUNICIPAL, 18.5590, 73.8070,
                    "Long-established core PMC area with mature piped infrastructure. General baseline, not from a specific dated 2026 source.",
                    GENERAL_KNOWLEDGE, null, "2026-07-01"),
            new Seed("Deccan Gymkhana", "Deccan", MUNICIPAL, 18.5162, 73.8412,
                    "Long-established core PMC area with mature piped infrastructure. General baseline, not from a specific dated 2026 source.",
                    GENERAL_KNOWLEDGE, null, "2026-07-01"),
            new Seed("Shivajinagar", "Shivajinagar", MUNICIPAL, 18.5304, 73.8567,
                    "Long-established core PMC area with mature piped infrastructure. General baseline, not from a specific dated 2026 source.",
                    GENERAL_KNOWLEDGE, null, "2026-07-01"),

            // 24x7 Water Supply Project: areas at/near completion
            new Seed("Ganeshnagar", null, MUNICIPAL, 18.4985, 73.8790,
                    "Water-meter installation under the 24x7 supply project has crossed 90% completion here, indicating piped infrastructure is largely in place.",
                    BUDGET_2026_27, BUDGET_2026_27_URL, "2026-03-09"),
            new Seed("Sukhsagarnagar", "Bibwewadi", MUNICIPAL, 18.4560, 73.8610,
                    "Water-meter installation under the 24x7 supply project has crossed 90% completion here, indicating piped infrastructure is largely in place.",
                    BUDGET_2026_27, BUDGET_2026_27_URL, "2026-03-09"),

            // Piped supply with recently revised timings (Bhama Askhed scheme)
            new Seed("Viman Nagar", "Viman Nagar", MIXED, 18.5679, 73.9143,
                    "Has a piped connection under the Bhama Askhed scheme (timings recently shifted to night/midnight/morning shifts), but also named in resident complaints about tanker dependency -- coverage is uneven across the locality.",
                    BHAMA_ASKHED, BHAMA_ASKHED_URL, "2025-12-21"),
            new Seed("Lohegaon", "Lohegaon", MIXED, 18.5820, 73.9280,
                    "Piped Bhama Askhed supply exists with recently revised shift timings, but the locality is also named among merged-village areas with tanker dependency and infrastructure neglect complaints.",
                    BHAMA_ASKHED, BHAMA_ASKHED_URL, "2025-12-21"),
            new Seed("Dhanori", "Dhanori", MIXED, 18.5730, 73.8990,
                    "Piped Bhama Askhed supply with recently revised shift timings (previously 6-11am, now split night/midnight/morning).",
                    "Punekar News", BHAMA_ASKHED_URL, "2025-07-31"),

            // Tanker-dependent localities
            new Seed("Wagholi", null, TANKER_DEPENDENT, 18.5793, 73.9812,
                    "Residents report continued tanker dependency despite paying municipal taxes; cited among East Pune areas lacking basic civic amenities.",
                    ELECTIONS_2026, ELECTIONS_2026_URL, "2025-12-21"),
            new Seed("Kharadi", null, TANKER_DEPENDENT, 18.5515, 73.9430,
                    "Named among East Pune localities with tanker dependency despite contributing heavily to city revenue.",
                    ELECTIONS_2026, ELECTIONS_2026_URL, "2025-12-21"),
            new Seed("Chandan Nagar", null, TANKER_DEPENDENT, 18.5563, 73.9310,
                    "Named among East Pune localities struggling with tanker dependency alongside broken roads and poor drainage.",
                    ELECTIONS_2026, ELECTIONS_2026_URL, "2025-12-21"),
            new Seed("Keshav Nagar (Mundhwa)", "Ward 41", TANKER_DEPENDENT, 18.5390, 73.9330,
                    "Residents describe daily traffic plus tanker dependency, with new building permissions granted ahead of basic infrastructure.",
                    ELECTIONS_2026, ELECTIONS_2026_URL, "2025-12-21"),
            new Seed("Undri", null, MIXED, 18.4370, 73.9250,
                    "Was tanker-dependent for years (residents describing feeling \"included on paper but excluded in reality\"); PMC water reached Undri for the first time in late May 2026 alongside Mohammed Wadi and NIBM, after new overhead tanks went live. Coverage is still rolling out in phases, and PMC also opened a new tanker filling point here in June 2026 to cover the gap in the meantime -- treat as transitional, not fully resolved.",
                    "Free Press Journal - Mohammed Wadi/Undri/NIBM Water Supply", "https://www.freepressjournal.in/pune/pune-mohammed-wadi-undri-and-nibm-residents-finally-get-pmc-water-supply-after-years-of-dependence-on-tankers", "2026-06-22"),
            new Seed("Fursungi", null, TANKER_DEPENDENT, 18.4650, 73.9550,
                    "Merged-village locality named among areas with longstanding civic neglect, including water supply.",
                    ELECTIONS_2026, ELECTIONS_2026_URL, "2025-12-21"),
            new Seed("Pisoli", null, TANKER_DEPENDENT, 18.4300, 73.8950,
                    "Recently merged into PMC but infrastructure still transitioning; developers typically arrange their own borewell/tanker supply rather than a municipal connection.",
                    REALTY_GUIDE, REALTY_GUIDE_URL, "2026-01-01"),
            new Seed("Ambegaon Budruk", null, TANKER_DEPENDENT, 18.4460, 73.8500,
                    "Recently merged into PMC but infrastructure still transitioning; often no municipal water connection, per property-buyer guidance.",
                    REALTY_GUIDE, REALTY_GUIDE_URL, "2026-01-01"),
            new Seed("Bavdhan Budruk", null, TANKER_DEPENDENT, 18.5090, 73.7730,
                    "Named among merged-village areas where residents feel excluded from PMC development benefits. PMC opened a new dedicated tanker filling point here in June 2026 in response to rising tanker demand, which corroborates continued dependency rather than resolving it.",
                    ELECTIONS_2026, ELECTIONS_2026_URL, "2026-06-22"),
            new Seed("Sus", null, TANKER_DEPENDENT, 18.5610, 73.7540,
                    "Named among merged-village areas where residents feel excluded from PMC development benefits.",
                    ELECTIONS_2026, ELECTIONS_2026_URL, "2025-12-21"),
            new Seed("Hinjewadi Phase 3", null, TANKER_DEPENDENT, 18.5910, 73.6900,
                    "Tanker dependency described as \"currently very real\" for new residential clusters here; largely outside mature PMC piped network.",
                    REALTY_GUIDE, REALTY_GUIDE_URL, "2026-01-01"),
            new Seed("Maan", null, TANKER_DEPENDENT, 18.5950, 73.7250,
                    "Newer micro-market at the PCMC/PMRDA periphery with weaker piped-supply infrastructure.",
                    REALTY_GUIDE, REALTY_GUIDE_URL, "2026-01-01"),
            new Seed("Marunji", null, TANKER_DEPENDENT, 18.5980, 73.7100,
                    "Newer micro-market at the PCMC/PMRDA periphery with weaker piped-supply infrastructure.",
                    REALTY_GUIDE, REALTY_GUIDE_URL, "2026-01-01"),

            // Pipeline / infrastructure work actively underway
            new Seed("Baner", "Aundh-Baner", PIPELINE_IN_PROGRESS, 18.5590, 73.7860,
                    "Longstanding water-shortage complaints (subject of an earlier Bombay High Court PIL); now within the PMC 24x7 water supply project's pipeline and storage-tank expansion, per the 2026-27 budget.",
                    BUDGET_2026_27, BUDGET_2026_27_URL, "2026-03-09"),
            new Seed("Balewadi", null, PIPELINE_IN_PROGRESS, 18.5680, 73.7730,
                    "Longstanding water-shortage complaints (subject of an earlier Bombay High Court PIL); now within the PMC 24x7 water supply project's pipeline and storage-tank expansion, per the 2026-27 budget.",
                    BUDGET_2026_27, BUDGET_2026_27_URL, "2026-03-09"),
            new Seed("Jambhulwadi", null, PIPELINE_IN_PROGRESS, 18.4460, 73.8250,
                    "Part of the merged-village cluster (with Wadachiwadi, Mangdewadi) budgeted for expanded PMC water supply as newly merged areas are brought onto the network.",
                    BUDGET_2026_27, BUDGET_2026_27_URL, "2026-03-09"),

            // More core PMC areas (added on request for broader coverage)
            new Seed("Hadapsar", null, MUNICIPAL, 18.5089, 73.9260,
                    "Established core PMC area. Typical PMC pattern applies: roughly 4-6 hours of piped supply per day on a rotational schedule, per property-buyer guidance -- not tied to a single dated 2026 news report.",
                    REALTY_GUIDE, REALTY_GUIDE_URL, "2026-01-01"),
            new Seed("Camp", null, MUNICIPAL, 18.5158, 73.8800,
                    "Established core PMC area with mature piped infrastructure, per property-buyer guidance.",
                    REALTY_GUIDE, REALTY_GUIDE_URL, "2026-01-01"),
            new Seed("Kalyani Nagar", null, MUNICIPAL, 18.5490, 73.9010,
                    "Established core PMC area with mature piped infrastructure, per property-buyer guidance.",
                    REALTY_GUIDE, REALTY_GUIDE_URL, "2026-01-01"),

            // Newly connected / transitional areas (May-June 2026)
            new Seed("Mohammadwadi", null, MIXED, 18.4550, 73.9300,
                    "PMC began the legal-connection process for societies here in May 2026 after new overhead tanks near Ghule Chowk went live; distribution was rolling out in phases over the following weeks after years of tanker dependence.",
                    "The Bridge Chronicle - PMC Legal Water Connections in Mohammadwadi", "https://www.thebridgechronicle.com/pune/pmc-legal-water-connections-mohammadwadi-new-tank-project-agn97", "2026-05-05"),
            new Seed("NIBM Road", null, MIXED, 18.4620, 73.9150,
                    "Received PMC piped water for the first time in late May 2026 alongside Mohammed Wadi and Undri, after years of tanker dependence; rollout still in an early phase.",
                    "Free Press Journal - Mohammed Wadi/Undri/NIBM Water Supply", "https://www.freepressjournal.in/pune/pune-mohammed-wadi-undri-and-nibm-residents-finally-get-pmc-water-supply-after-years-of-dependence-on-tankers", "2026-05-28"),

            // Under-covered tanker-heavy village
            new Seed("Uruli Devachi", null, TANKER_DEPENDENT, 18.4550, 73.9700,
                    "Heavy, continuing tanker dependency; PMC spent roughly Rs 9 crore on tanker operations here and in adjoining Phursungi in FY2025-26 alone, per RTI data, with residents citing a lack of permanent pipeline infrastructure and contaminated local groundwater near the old garbage depot.",
                    RTI_TANKER_SPEND, RTI_TANKER_SPEND_URL, "2026-05-22"),

            // Southern/eastern localities with reported seasonal tanker-demand surges (summer 2026)
            new Seed("Dhankawadi", null, MIXED, 18.4650, 73.8500,
                    "Piped PMC supply exists but the area was named among localities with a sharp rise in tanker requests during the 2026 summer months.",
                    SUMMER_TANKER_SURGE, SUMMER_TANKER_SURGE_URL, "2026-03-13"),
            new Seed("Katraj", null, MIXED, 18.4570, 73.8590,
                    "Piped PMC supply exists but the area was named among localities with a sharp rise in tanker requests during the 2026 summer months.",
                    SUMMER_TANKER_SURGE, SUMMER_TANKER_SURGE_URL, "2026-03-13"),
            new Seed("Kondhwa", null, MIXED, 18.4640, 73.8930,
                    "Piped PMC supply exists but the area was named among localities with a sharp rise in tanker requests during the 2026 summer months.",
                    SUMMER_TANKER_SURGE, SUMMER_TANKER_SURGE_URL, "2026-03-13"),
            new Seed("Yewalewadi", null, MIXED, 18.4390, 73.8600,
                    "Piped PMC supply exists but the area was named among localities with a sharp rise in tanker requests during the 2026 summer months.",
                    SUMMER_TANKER_SURGE, SUMMER_TANKER_SURGE_URL, "2026-03-13"),
            new Seed("Kirkatwadi", null, MIXED, 18.4460, 73.7920,
                    "Piped PMC supply exists but the area was named among localities with a sharp rise in tanker requests during the 2026 summer months.",
                    SUMMER_TANKER_SURGE, SUMMER_TANKER_SURGE_URL, "2026-03-13"),
            new Seed("Vadgaon Budruk", null, MIXED, 18.4650, 73.7960,
                    "Piped PMC supply exists but the area was named among localities with a sharp rise in tanker requests during the 2026 summer months.",
                    SUMMER_TANKER_SURGE, SUMMER_TANKER_SURGE_URL, "2026-03-13"),
            new Seed("Dhayari", null, MIXED, 18.4530, 73.8180,
                    "Piped PMC supply exists but the area was named among localities with a sharp rise in tanker requests during the 2026 summer months.",
                    SUMMER_TANKER_SURGE, SUMMER_TANKER_SURGE_URL, "2026-03-13"),
            new Seed("Pashan", null, MIXED, 18.5390, 73.7870,
                    "Piped PMC supply exists but the area was named among localities with a sharp rise in tanker requests during the 2026 summer months.",
                    SUMMER_TANKER_SURGE, SUMMER_TANKER_SURGE_URL, "2026-03-13")
    );
}
