package org.punewatertracker.security;

import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;
import org.springframework.http.HttpMethod;
import org.springframework.security.authentication.AuthenticationManager;
import org.springframework.security.authentication.dao.DaoAuthenticationProvider;
import org.springframework.security.config.annotation.authentication.configuration.AuthenticationConfiguration;
import org.springframework.security.config.annotation.web.builders.HttpSecurity;
import org.springframework.security.config.annotation.web.configuration.EnableWebSecurity;
import org.springframework.security.config.http.SessionCreationPolicy;
import org.springframework.security.crypto.bcrypt.BCryptPasswordEncoder;
import org.springframework.security.crypto.password.PasswordEncoder;
import org.springframework.security.web.SecurityFilterChain;
import org.springframework.security.web.authentication.UsernamePasswordAuthenticationFilter;
import org.springframework.web.cors.CorsConfiguration;
import org.springframework.web.cors.CorsConfigurationSource;
import org.springframework.web.cors.UrlBasedCorsConfigurationSource;

import java.util.List;

@Configuration
@EnableWebSecurity
public class SecurityConfig {

    private final AppUserDetailsService userDetailsService;
    private final JwtAuthFilter jwtAuthFilter;

    public SecurityConfig(AppUserDetailsService userDetailsService, JwtAuthFilter jwtAuthFilter) {
        this.userDetailsService = userDetailsService;
        this.jwtAuthFilter = jwtAuthFilter;
    }

    @Bean
    public PasswordEncoder passwordEncoder() {
        return new BCryptPasswordEncoder();
    }

    @Bean
    public DaoAuthenticationProvider authenticationProvider() {
        DaoAuthenticationProvider provider = new DaoAuthenticationProvider();
        provider.setUserDetailsService(userDetailsService);
        provider.setPasswordEncoder(passwordEncoder());
        return provider;
    }

    @Bean
    public AuthenticationManager authenticationManager(AuthenticationConfiguration config) throws Exception {
        return config.getAuthenticationManager();
    }

    @Bean
    public SecurityFilterChain filterChain(HttpSecurity http) throws Exception {
        http
                .cors(cors -> cors.configurationSource(corsConfigurationSource()))
                .csrf(csrf -> csrf.disable()) // stateless API, token in header, not a cookie -- CSRF doesn't apply
                .sessionManagement(session -> session.sessionCreationPolicy(SessionCreationPolicy.STATELESS))
                .authorizeHttpRequests(auth -> auth
                        // H2 console, dev convenience only
                        .requestMatchers("/h2-console/**").permitAll()

                        // Public RSS feed of recent status updates
                        .requestMatchers(HttpMethod.GET, "/rss.xml").permitAll()

                        // Anyone can attempt to log in
                        .requestMatchers("/api/auth/login").permitAll()

                        // Public read access
                        .requestMatchers(HttpMethod.GET, "/api/localities/**").permitAll()

                        // MCP: read-only tool access for AI assistants, same trust level as
                        // the public GET endpoints above -- it wraps the exact same read methods
                        .requestMatchers("/mcp/**").permitAll()

                        // Ward lookup is read-only public data; recomputing/persisting a ward
                        // on a locality is the same trust level as editing that locality
                        .requestMatchers(HttpMethod.GET, "/api/wards/**").permitAll()
                        .requestMatchers(HttpMethod.PUT, "/api/wards/**").hasAnyRole("ADMIN", "EDITOR")

                        // Citizens can submit reports without logging in; they land unverified
                        .requestMatchers(HttpMethod.POST, "/api/localities/reports").permitAll()

                        // Only admins manage user accounts
                        .requestMatchers("/api/users/**").hasRole("ADMIN")

                        // Only admins can flip runtime feature toggles
                        .requestMatchers("/api/admin/features/**").hasRole("ADMIN")

                        // Only admins can view the audit trail
                        .requestMatchers("/api/audit-log/**").hasRole("ADMIN")

                        // Only admins can control Chaos Monkey (when the chaos-monkey profile is active)
                        .requestMatchers("/actuator/chaosmonkey/**").hasRole("ADMIN")

                        // Only admins can delete entries
                        .requestMatchers(HttpMethod.DELETE, "/api/localities/**").hasRole("ADMIN")

                        // Admins and editors can create/edit entries and approve reports
                        .requestMatchers(HttpMethod.POST, "/api/localities/**").hasAnyRole("ADMIN", "EDITOR")
                        .requestMatchers(HttpMethod.PUT, "/api/localities/**").hasAnyRole("ADMIN", "EDITOR")

                        .anyRequest().authenticated()
                )
                .headers(headers -> headers.frameOptions(frame -> frame.disable())) // needed for H2 console
                .authenticationProvider(authenticationProvider())
                .addFilterBefore(jwtAuthFilter, UsernamePasswordAuthenticationFilter.class);

        return http.build();
    }

    @Bean
    public CorsConfigurationSource corsConfigurationSource() {
        CorsConfiguration configuration = new CorsConfiguration();
        configuration.setAllowedOriginPatterns(List.of(
                "http://localhost:*",
                "https://pmc-water-tracker.vercel.app"
        ));
        configuration.setAllowedMethods(List.of("GET", "POST", "PUT", "DELETE", "OPTIONS"));
        configuration.setAllowedHeaders(List.of("*"));

        UrlBasedCorsConfigurationSource source = new UrlBasedCorsConfigurationSource();
        source.registerCorsConfiguration("/**", configuration);
        return source;
    }
}
