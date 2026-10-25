package org.punewatertracker.controller;

import jakarta.servlet.http.HttpServletResponse;
import org.punewatertracker.model.Locality;
import org.punewatertracker.repository.LocalityRepository;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.RestController;

import javax.xml.stream.XMLOutputFactory;
import javax.xml.stream.XMLStreamWriter;
import java.io.IOException;
import java.time.LocalDate;
import java.time.ZoneOffset;
import java.time.format.DateTimeFormatter;
import java.util.List;

/**
 * RSS 2.0 feed of recent water-status updates -- lets people subscribe via any feed reader
 * and get notified when localities change status, without needing to check the site.
 *
 * Hand-rolled via the JDK's built-in javax.xml.stream.XMLStreamWriter rather than a library
 * (ROME): this project's search for current ROME Maven coordinates turned up conflicting
 * version numbers from different sources, the exact class of uncertainty that caused real
 * pain earlier with the MCP SDK. RSS 2.0 is simple enough that XMLStreamWriter -- which
 * handles XML escaping correctly and automatically, important since this includes
 * user-submitted content (locality notes) -- covers this without that risk.
 */
@RestController
public class RssFeedController {

    private static final int MAX_ITEMS = 30;
    private static final String SITE_URL = "https://pmc-water-tracker.vercel.app";

    private final LocalityRepository repository;

    public RssFeedController(LocalityRepository repository) {
        this.repository = repository;
    }

    @GetMapping(value = "/rss.xml")
    public void feed(HttpServletResponse response) throws IOException {
        response.setContentType("application/rss+xml; charset=UTF-8");

        List<Locality> recent = repository.findByVerifiedTrueOrderByLastVerifiedDesc().stream()
                .limit(MAX_ITEMS)
                .toList();

        try {
            XMLOutputFactory factory = XMLOutputFactory.newInstance();
            XMLStreamWriter xml = factory.createXMLStreamWriter(response.getWriter());

            xml.writeStartDocument("UTF-8", "1.0");
            xml.writeStartElement("rss");
            xml.writeAttribute("version", "2.0");

            xml.writeStartElement("channel");
            writeTextElement(xml, "title", "Where the Water Reaches -- PMC Ward Register");
            writeTextElement(xml, "link", SITE_URL);
            writeTextElement(xml, "description",
                    "Recent water supply status updates across Pune Municipal Corporation localities.");
            writeTextElement(xml, "language", "en-in");

            for (Locality locality : recent) {
                writeItem(xml, locality);
            }

            xml.writeEndElement(); // channel
            xml.writeEndElement(); // rss
            xml.writeEndDocument();
            xml.flush();
            xml.close();
        } catch (Exception ex) {
            throw new IOException("Failed to generate RSS feed", ex);
        }
    }

    private void writeItem(XMLStreamWriter xml, Locality locality) throws Exception {
        xml.writeStartElement("item");

        writeTextElement(xml, "title", locality.getName() + ": " + locality.getStatus());

        String itemLink = SITE_URL + "/?locality=" + locality.getId();
        writeTextElement(xml, "link", itemLink);

        String description = locality.getNotes() != null && !locality.getNotes().isBlank()
                ? locality.getNotes()
                : "Status updated to " + locality.getStatus() + ".";
        writeTextElement(xml, "description", description);

        // guid uniquely identifies this item to feed readers so they don't show it twice;
        // isPermaLink=false since this isn't a real dereferenceable URL for the item itself.
        xml.writeStartElement("guid");
        xml.writeAttribute("isPermaLink", "false");
        xml.writeCharacters("locality-" + locality.getId() + "-" + locality.getLastVerified());
        xml.writeEndElement();

        if (locality.getLastVerified() != null) {
            writeTextElement(xml, "pubDate", toRfc822(locality.getLastVerified()));
        }

        xml.writeEndElement(); // item
    }

    private void writeTextElement(XMLStreamWriter xml, String name, String text) throws Exception {
        xml.writeStartElement(name);
        xml.writeCharacters(text != null ? text : "");
        xml.writeEndElement();
    }

    /** RSS 2.0 requires RFC 822 date format for pubDate. lastVerified is only a LocalDate (no
     *  time-of-day is tracked), so this treats it as midnight UTC on that date -- an
     *  approximation, not a precise timestamp, but the actual data available. */
    private String toRfc822(LocalDate date) {
        return date.atStartOfDay(ZoneOffset.UTC).format(DateTimeFormatter.RFC_1123_DATE_TIME);
    }
}
