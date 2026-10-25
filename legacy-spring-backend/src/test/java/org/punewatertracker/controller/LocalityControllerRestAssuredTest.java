package org.punewatertracker.controller;

import io.restassured.RestAssured;
import io.restassured.http.ContentType;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.springframework.boot.test.context.SpringBootTest;
import org.springframework.boot.test.web.server.LocalServerPort;
import org.springframework.test.context.ActiveProfiles;

import static io.restassured.RestAssured.given;
import static org.hamcrest.Matchers.*;

/**
 * REST Assured tests the REAL HTTP layer end to end -- routing, Spring Security's actual
 * filter chain, JSON serialization, ApiExceptionHandler -- against a genuinely running
 * embedded server. This is precisely the case from the earlier "when do you need Spring
 * context" discussion where you DO need it: no mock-based unit test could ever have caught
 * the Chaos-Monkey-discovered /error 403 bug, because that lived entirely in the interaction
 * between Tomcat's error handling and the real Security filter chain -- exactly what these
 * tests actually exercise.
 *
 * WEB_ENVIRONMENT_RANDOM_PORT starts a real embedded Tomcat on a random free port, avoiding
 * conflicts if tests ever run in parallel or alongside a locally-running dev instance.
 */
@SpringBootTest(webEnvironment = SpringBootTest.WebEnvironment.RANDOM_PORT)
@ActiveProfiles("dev") // H2, same convention as every other integration test in this project
class LocalityControllerRestAssuredTest {

    @LocalServerPort
    private int port;

    @BeforeEach
    void setUp() {
        RestAssured.baseURI = "http://localhost";
        RestAssured.port = port;
    }

    @Test
    void listLocalities_publicEndpoint_returns200WithJsonArray() {
        given()
        .when()
            .get("/api/localities")
        .then()
            .statusCode(200)
            .contentType(ContentType.JSON)
            .body("$", isA(java.util.List.class));
    }

    @Test
    void listLocalities_filterByInvalidStatus_returns400() {
        // WaterStatus is an enum -- a value it can't parse should surface as a real 400,
        // via ApiExceptionHandler's MethodArgumentNotValidException / type-mismatch handling,
        // not a raw 500. Confirms the real error-handling path, not just the happy path.
        given()
            .queryParam("status", "NOT_A_REAL_STATUS")
        .when()
            .get("/api/localities")
        .then()
            .statusCode(400);
    }

    @Test
    void submitCitizenReport_publicEndpoint_createsUnverifiedReport() {
        String requestBody = """
            {
              "name": "Test Locality via REST Assured",
              "status": "TANKER_DEPENDENT",
              "latitude": 18.55,
              "longitude": 73.85,
              "citizenDescription": "No water for three days"
            }
            """;

        given()
            .contentType(ContentType.JSON)
            .body(requestBody)
        .when()
            .post("/api/localities/reports")
        .then()
            .statusCode(200)
            .body("verified", equalTo(false)) // citizen reports never go live immediately
            .body("name", equalTo("Test Locality via REST Assured"));
    }

    @Test
    void login_wrongPassword_returns401NotAStackTrace() {
        String requestBody = """
            {"username": "admin", "password": "definitely-wrong-password"}
            """;

        given()
            .contentType(ContentType.JSON)
            .body(requestBody)
        .when()
            .post("/api/auth/login")
        .then()
            .statusCode(401);
    }

    @Test
    void adminOnlyEndpoint_withoutToken_isRejected() {
        String requestBody = """
            {
              "name": "Should Never Be Created",
              "status": "MUNICIPAL",
              "latitude": 18.5,
              "longitude": 73.8
            }
            """;

        given()
            .contentType(ContentType.JSON)
            .body(requestBody)
        .when()
            .post("/api/localities") // admin-only create-verified endpoint, no auth header at all
        .then()
            .statusCode(403);
    }

    @Test
    void loginThenCreateVerifiedLocality_fullAuthenticatedFlow() {
        // Step 1: log in with the real seeded admin account (see AdminBootstrap) to get a
        // real, valid JWT -- not a fabricated one, the actual token this app would issue.
        String loginBody = """
            {"username": "admin", "password": "change-me-now"}
            """;

        String token =
            given()
                .contentType(ContentType.JSON)
                .body(loginBody)
            .when()
                .post("/api/auth/login")
            .then()
                .statusCode(200)
                .extract()
                .path("token");

        // Step 2: use that real token to hit the admin-only endpoint -- proves the WHOLE
        // chain works together: login issues a valid token, JwtAuthFilter accepts it,
        // Security's role check passes for ADMIN, the controller actually runs.
        String createBody = """
            {
              "name": "REST Assured Verified Test",
              "status": "MUNICIPAL",
              "latitude": 18.52,
              "longitude": 73.85,
              "sourceName": "Test suite",
              "sourceUrl": "https://example.com"
            }
            """;

        given()
            .header("Authorization", "Bearer " + token)
            .contentType(ContentType.JSON)
            .body(createBody)
        .when()
            .post("/api/localities")
        .then()
            .statusCode(200)
            .body("verified", equalTo(true))
            .body("name", equalTo("REST Assured Verified Test"));
    }
}
