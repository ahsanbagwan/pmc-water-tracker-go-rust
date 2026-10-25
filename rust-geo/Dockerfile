FROM rust:1-slim AS build
WORKDIR /src
COPY Cargo.toml Cargo.lock ./
COPY src ./src
RUN cargo build --release

FROM gcr.io/distroless/cc-debian12:nonroot
COPY --from=build /src/target/release/pmc-geo /pmc-geo
# Optional: mount the ward boundary file and set WARD_GEOJSON (default: static/pune-admin-wards.geojson)
EXPOSE 8090
USER nonroot
ENTRYPOINT ["/pmc-geo"]
